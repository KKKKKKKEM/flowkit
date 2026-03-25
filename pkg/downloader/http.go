package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/proxy"

	"github.com/KKKKKKKEM/grasp/pkg/core"
)

const (
	MetaKeyProxy     = "proxy"      // "http://..." | "socks5://..."
	MetaKeyChunkSize = "chunk_size" // int64 bytes, default 4MB
)

const defaultChunkSize int64 = 4 * 1024 * 1024

// chunkState 记录单个分片的下载状态，序列化到 <filename>.grasp 文件实现断点续传。
type chunkState struct {
	Index int   `json:"index"`
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  bool  `json:"done"`
}

type resumeMeta struct {
	URL    string       `json:"url"`
	Total  int64        `json:"total"`
	Chunks []chunkState `json:"chunks"`
}

type HTTPDownloader struct{}

func (d *HTTPDownloader) Name() string { return "http" }

func (d *HTTPDownloader) CanHandle(task *core.DownloadTask) bool {
	u := strings.ToLower(task.URL)
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

func (d *HTTPDownloader) Download(ctx context.Context, task *core.DownloadTask) (*core.DownloadResult, error) {
	maxAttempts := task.Retry + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	retryInterval := task.RetryInterval
	if retryInterval <= 0 {
		retryInterval = time.Second
	}

	var (
		result *core.DownloadResult
		err    error
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err = d.download(ctx, task)
		if err == nil {
			return result, nil
		}
		if attempt == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryInterval):
		}
	}
	return nil, err
}

func (d *HTTPDownloader) download(ctx context.Context, task *core.DownloadTask) (*core.DownloadResult, error) {
	client, err := buildClient(task)
	if err != nil {
		return nil, err
	}

	total, supportsRange, err := probe(ctx, client, task)
	if err != nil {
		return nil, err
	}

	destPath := resolveDestPath(task)

	concurrency := task.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}

	var written int64
	if total <= 0 || !supportsRange || concurrency == 1 {
		written, err = downloadSingle(ctx, client, task, destPath)
	} else {
		written, err = downloadChunked(ctx, client, task, destPath, total, concurrency)
	}
	if err != nil {
		return nil, err
	}

	return &core.DownloadResult{FilePath: destPath, BytesWritten: written}, nil
}

func buildClient(task *core.DownloadTask) (*http.Client, error) {
	transport := &http.Transport{}

	proxyRaw := task.Proxy
	if proxyRaw == "" {
		proxyRaw, _ = task.StringMeta(MetaKeyProxy)
	}

	switch {
	case proxyRaw == "env":
		transport.Proxy = http.ProxyFromEnvironment
	case proxyRaw != "":
		proxyURL, err := url.Parse(proxyRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", proxyRaw, err)
		}
		switch strings.ToLower(proxyURL.Scheme) {
		case "http", "https":
			transport.Proxy = http.ProxyURL(proxyURL)
		case "socks5", "socks5h":
			if err := applySocks5(transport, proxyURL); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported proxy scheme %q", proxyURL.Scheme)
		}
	}

	return &http.Client{Transport: transport, Timeout: task.Timeout}, nil
}

func applySocks5(transport *http.Transport, proxyURL *url.URL) error {
	var auth *proxy.Auth
	if proxyURL.User != nil {
		pass, _ := proxyURL.User.Password()
		auth = &proxy.Auth{User: proxyURL.User.Username(), Password: pass}
	}
	dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, auth, proxy.Direct)
	if err != nil {
		return fmt.Errorf("socks5 dialer: %w", err)
	}
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.Dial(network, addr)
	}
	return nil
}

func probe(ctx context.Context, client *http.Client, task *core.DownloadTask) (total int64, supportsRange bool, err error) {
	req, err := newRequest(ctx, http.MethodHead, task)
	if err != nil {
		return 0, false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false, fmt.Errorf("HEAD %s: %w", task.URL, err)
	}
	resp.Body.Close()

	return resp.ContentLength, resp.Header.Get("Accept-Ranges") == "bytes", nil
}

func resolveDestPath(task *core.DownloadTask) string {
	info, err := os.Stat(task.Dest)
	if err == nil && info.IsDir() {
		return filepath.Join(task.Dest, task.FilenameFromURL())
	}
	return task.Dest
}

func downloadSingle(ctx context.Context, client *http.Client, task *core.DownloadTask, destPath string) (int64, error) {
	req, err := newRequest(ctx, http.MethodGet, task)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	return copyWithProgress(f, resp.Body, -1, task.OnProgress)
}

func downloadChunked(ctx context.Context, client *http.Client, task *core.DownloadTask, destPath string, total int64, concurrency int) (int64, error) {
	chunkSize := defaultChunkSize
	if cs, ok := task.Int64Meta(MetaKeyChunkSize); ok && cs > 0 {
		chunkSize = cs
	}

	metaPath := destPath + ".grasp"
	rm, err := loadOrBuildResumeMeta(metaPath, task.URL, total, chunkSize)
	if err != nil {
		return 0, err
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	if err := f.Truncate(total); err != nil {
		return 0, fmt.Errorf("preallocate %s: %w", destPath, err)
	}

	written, err := runChunkWorkers(ctx, client, task, f, rm, metaPath, total, concurrency)
	if err != nil {
		return 0, err
	}

	os.Remove(metaPath)
	return written, nil
}

func runChunkWorkers(ctx context.Context, client *http.Client, task *core.DownloadTask, f *os.File, rm *resumeMeta, metaPath string, total int64, concurrency int) (int64, error) {
	var completedBytes atomic.Int64
	for _, c := range rm.Chunks {
		if c.Done {
			completedBytes.Add(c.End - c.Start + 1)
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var metaMu sync.Mutex
	sem := make(chan struct{}, concurrency)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup

	for i := range rm.Chunks {
		if rm.Chunks[i].Done {
			continue
		}
		select {
		case <-ctx.Done():
			wg.Wait()
			return 0, ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(c *chunkState) {
			defer wg.Done()
			defer func() { <-sem }()

			n, err := downloadChunk(ctx, client, task, f, c)
			if err != nil {
				select {
				case errCh <- err:
					cancel()
				default:
				}
				return
			}

			completedBytes.Add(n)
			if task.OnProgress != nil {
				task.OnProgress(completedBytes.Load(), total)
			}
			metaMu.Lock()
			c.Done = true
			// 忽略元数据写入错误，不中断下载
			_ = saveResumeMeta(metaPath, rm)
			metaMu.Unlock()
		}(&rm.Chunks[i])
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return 0, err
	default:
		return completedBytes.Load(), nil
	}
}

func downloadChunk(ctx context.Context, client *http.Client, task *core.DownloadTask, f *os.File, c *chunkState) (int64, error) {
	req, err := newRequest(ctx, http.MethodGet, task)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", c.Start, c.End))

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("chunk %d: %w", c.Index, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("chunk %d: unexpected status %d", c.Index, resp.StatusCode)
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("chunk %d read: %w", c.Index, err)
	}
	// WriteAt 是并发安全的（各分片写不同偏移量，互不重叠）
	n, err := f.WriteAt(buf, c.Start)
	return int64(n), err
}

func loadOrBuildResumeMeta(metaPath, rawURL string, total, chunkSize int64) (*resumeMeta, error) {
	if data, err := os.ReadFile(metaPath); err == nil {
		var rm resumeMeta
		if json.Unmarshal(data, &rm) == nil && rm.URL == rawURL && rm.Total == total {
			return &rm, nil
		}
	}

	rm := &resumeMeta{URL: rawURL, Total: total, Chunks: buildChunks(total, chunkSize)}
	if err := saveResumeMeta(metaPath, rm); err != nil {
		return nil, err
	}
	return rm, nil
}

func buildChunks(total, chunkSize int64) []chunkState {
	var chunks []chunkState
	for i, start := 0, int64(0); start < total; i++ {
		end := start + chunkSize - 1
		if end > total-1 {
			end = total - 1
		}
		chunks = append(chunks, chunkState{Index: i, Start: start, End: end})
		start = end + 1
	}
	return chunks
}

func saveResumeMeta(path string, rm *resumeMeta) error {
	data, err := json.Marshal(rm)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func newRequest(ctx context.Context, method string, task *core.DownloadTask) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, task.URL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range task.Headers {
		req.Header.Set(k, v)
	}
	return req, nil
}

func copyWithProgress(dst io.Writer, src io.Reader, total int64, onProgress core.ProgressFunc) (int64, error) {
	if onProgress == nil {
		return io.Copy(dst, src)
	}
	var written atomic.Int64
	r := io.TeeReader(src, writerFunc(func(p []byte) (int, error) {
		n, err := dst.Write(p)
		written.Add(int64(n))
		onProgress(written.Load(), total)
		return n, err
	}))
	_, err := io.Copy(io.Discard, r)
	return written.Load(), err
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
