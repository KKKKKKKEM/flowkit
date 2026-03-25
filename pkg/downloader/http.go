package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/KKKKKKKEM/grasp/pkg/core"
)

const defaultChunkSize int64 = 1 * 1024 * 1024

type SimpleHTTPDownloader struct{}

func (d *SimpleHTTPDownloader) Name() string {
	return "simple-http"
}

func (d *SimpleHTTPDownloader) CanHandle(task *core.DownloadTask) bool {
	u := strings.ToLower(task.URL)
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

func (d *SimpleHTTPDownloader) Download(ctx context.Context, task *core.DownloadTask) (*core.DownloadResult, error) {
	result, err := d.download(ctx, task)
	if err != nil {
		if task.OnError != nil {
			task.OnError(err)
		}
		return nil, err
	}
	if task.OnComplete != nil {
		task.OnComplete(result)
	}
	return result, nil
}

func (d *SimpleHTTPDownloader) Stream(ctx context.Context, task *core.DownloadTask) (io.ReadCloser, error) {
	client := buildClient(task)
	req, err := newRequest(ctx, http.MethodGet, task.URL, task.Headers)
	if err != nil {
		return nil, err
	}
	resp, err := doWithRetry(client, req, task.Retry, task.Interval())
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	return resp.Body, nil
}

func (d *SimpleHTTPDownloader) download(ctx context.Context, task *core.DownloadTask) (*core.DownloadResult, error) {
	client := buildClient(task)

	totalSize, acceptsRanges := probeContentLength(ctx, client, task)

	concurrency := task.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	dest, err := task.RealDest()
	if err != nil {
		return nil, err
	}

	f, err := os.Create(dest)
	if err != nil {
		return nil, fmt.Errorf("create file %s: %w", dest, err)
	}
	defer f.Close()

	// Range 并发分块：需服务端支持 Accept-Ranges: bytes
	if concurrency <= 1 || !acceptsRanges || totalSize <= 0 {
		concurrency = 1
	}
	written, err := runSegments(ctx, client, task, f, totalSize, concurrency)
	if err != nil {
		return nil, err
	}
	return &core.DownloadResult{FilePath: dest, BytesWritten: written}, nil
}

type writeCmd struct {
	offset int64
	buf    []byte
}

// runSegments splits [0, totalSize) into `concurrency` segments, launches one
// producer goroutine per segment, and drains all writeCmd values through a
// single consumer that calls f.WriteAt. When totalSize <= 0 the file size is
// unknown and a single segment covering the whole response is used instead.
func runSegments(ctx context.Context, client *http.Client, task *core.DownloadTask, f *os.File, totalSize int64, concurrency int) (int64, error) {
	chunkSize := task.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}

	type segment struct {
		start int64 // byte offset in the file (used for WriteAt and Range header)
		end   int64 // inclusive; -1 means "no Range header, stream to EOF"
	}

	var segments []segment
	if totalSize > 0 && concurrency > 1 {
		for start := int64(0); start < totalSize; start += chunkSize {
			end := start + chunkSize - 1
			if end >= totalSize {
				end = totalSize - 1
			}
			segments = append(segments, segment{start: start, end: end})
		}
	} else {
		segments = []segment{{start: 0, end: -1}}
	}

	// Channel capacity = concurrency so fast producers don't stall waiting for
	// the consumer while still bounding in-flight memory to ~concurrency chunks.
	cmds := make(chan writeCmd, concurrency)

	var (
		wg         sync.WaitGroup
		downloaded atomic.Int64
		firstErr   atomic.Value
	)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, concurrency)

	for _, seg := range segments {
		wg.Add(1)
		sem <- struct{}{}
		go func(seg segment) {
			defer wg.Done()
			defer func() { <-sem }()

			if firstErr.Load() != nil {
				return
			}

			err := produceSegment(ctx, client, task, seg.start, seg.end, cmds)
			if err != nil && firstErr.CompareAndSwap(nil, err) {
				cancel()
			}
		}(seg)
	}

	go func() {
		wg.Wait()
		close(cmds)
	}()

	var written int64
	for cmd := range cmds {
		nw, writeErr := f.WriteAt(cmd.buf, cmd.offset)
		written += int64(nw)
		if writeErr != nil {
			firstErr.CompareAndSwap(nil, fmt.Errorf("write at %d: %w", cmd.offset, writeErr))
			cancel()
		}
		if task.OnProgress != nil {
			task.OnProgress(downloaded.Add(int64(len(cmd.buf))), totalSize)
		}
	}

	if v := firstErr.Load(); v != nil {
		return 0, v.(error)
	}
	return written, nil
}

// produceSegment streams one HTTP segment (Range or full) and sends writeCmd
// values to cmds. start is the file offset; end == -1 means no Range header.
func produceSegment(ctx context.Context, client *http.Client, task *core.DownloadTask, start, end int64, cmds chan<- writeCmd) error {
	req, err := newRequest(ctx, http.MethodGet, task.URL, task.Headers)
	if err != nil {
		return err
	}
	if end >= 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	}

	resp, err := doWithRetry(client, req, task.Retry, task.Interval())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if end >= 0 && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("segment %d-%d: expected 206, got %d", start, end, resp.StatusCode)
	}
	if end < 0 && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	offset := start
	buf := make([]byte, 32*1024)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			// Copy the slice: the producer must not reuse buf until the consumer
			// has finished with the previous payload.
			payload := make([]byte, n)
			copy(payload, buf[:n])
			select {
			case cmds <- writeCmd{offset: offset, buf: payload}:
			case <-ctx.Done():
				return ctx.Err()
			}
			offset += int64(n)
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read segment %d: %w", start, readErr)
		}
	}
}

// doWithRetry reconstructs the request on each retry because the response body
// of a prior attempt may have been partially consumed.
func doWithRetry(client *http.Client, req *http.Request, maxRetry int, interval time.Duration) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetry; attempt++ {
		if attempt > 0 {
			newReq, err := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), nil)
			if err != nil {
				return nil, err
			}
			newReq.Header = req.Header
			req = newReq
			time.Sleep(interval)
		}
		resp, err := client.Do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("after %d attempt(s): %w", maxRetry+1, lastErr)
}

func buildClient(task *core.DownloadTask) *http.Client {
	transport := &http.Transport{}

	switch task.Proxy {
	case "":
		transport.Proxy = nil
	case "env":
		transport.Proxy = http.ProxyFromEnvironment
	default:
		proxyURL, err := url.Parse(task.Proxy)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &http.Client{
		Timeout:   task.Timeout,
		Transport: transport,
	}
}

func probeContentLength(ctx context.Context, client *http.Client, task *core.DownloadTask) (int64, bool) {
	req, err := newRequest(ctx, http.MethodHead, task.URL, task.Headers)
	if err != nil {
		return -1, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return -1, false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return -1, false
	}

	acceptsRanges := strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes")
	return resp.ContentLength, acceptsRanges
}

func newRequest(ctx context.Context, method, rawURL string, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}
