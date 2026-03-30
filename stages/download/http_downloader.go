package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type Requester interface {
	Name() string
	Request(ctx context.Context, req *http.Request, opts *Opts) (*http.Response, error)
}

const defaultChunkSize int64 = 1 * 1024 * 1024

type BaseHTTPDownloader struct {
	requester Requester
}

type WriteCmd struct {
	segIdx int
	offset int64
	buf    []byte
}

func ensureTaskOpts(task *Task) *Opts {
	if task.Opts == nil {
		task.Opts = &Opts{}
	}
	return task.Opts
}

func (d *BaseHTTPDownloader) CanHandle(task *Task) bool {
	if task == nil || task.Request == nil || task.Request.URL == nil {
		return false
	}
	scheme := task.Request.URL.Scheme
	return scheme == "http" || scheme == "https"
}

func (d *BaseHTTPDownloader) Probe(ctx context.Context, task *Task) (int64, bool, http.Header) {
	opts := ensureTaskOpts(task)
	headRequest := task.Request.Clone(ctx)
	headRequest.Method = http.MethodHead

	resp, err := d.requester.Request(ctx, headRequest, opts)
	if err != nil {
		return -1, false, nil
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return -1, false, nil
	}

	acceptsRanges := strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes")
	return resp.ContentLength, acceptsRanges, resp.Header
}

func (d *BaseHTTPDownloader) buildSegments(totalSize, chunkSize int64, concurrency int) []Segment {
	if totalSize > 0 && concurrency > 1 {
		var segments []Segment
		for start := int64(0); start < totalSize; start += chunkSize {
			end := start + chunkSize - 1
			if end >= totalSize {
				end = totalSize - 1
			}
			segments = append(segments, Segment{Start: start, End: end, Idx: int(start / chunkSize)})
		}
		return segments
	}
	return []Segment{{Start: 0, End: -1}}
}

func (d *BaseHTTPDownloader) Download(ctx context.Context, task *Task) (*Result, error) {
	if task == nil || task.Request == nil {
		return nil, fmt.Errorf("task or request is nil")
	}
	_ = ensureTaskOpts(task)

	totalSize, acceptsRanges, probeHeader := d.Probe(ctx, task)
	if err := patchPath(task, probeHeader); err != nil {
		return nil, err
	}

	dest := task.SavePath
	tmp := dest + ".tmp"
	chunkSize := task.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}

	concurrency := task.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency <= 1 || !acceptsRanges || totalSize <= 0 {
		concurrency = 1
	}

	allSegments := d.buildSegments(totalSize, chunkSize, concurrency)
	var meta *Meta

	switch {

	case task.Overwrite: // 重新下载
		// 删除缓存文件
		_ = os.Remove(dest)
		_ = os.Remove(tmp)
		_ = task.RemoveMeta()

	case FileExistsAt(dest): // 已经下载完了
		if info, err := os.Stat(dest); err == nil {
			return &Result{Path: dest, Size: info.Size()}, nil
		}

	case FileExistsAt(tmp): // 下载了一部分
		if m, err := task.LoadMeta(); err == nil &&
			m.TotalSize == totalSize &&
			m.ChunkSize == chunkSize &&
			len(m.Segments) == len(allSegments) {
			meta = m
		}
	}

	if meta == nil {
		_ = os.Remove(tmp)
		_ = task.RemoveMeta()
		meta = &Meta{
			TotalSize: totalSize,
			ChunkSize: chunkSize,
			Segments:  allSegments,
		}
		if err := task.SaveMeta(meta); err != nil {
			return nil, fmt.Errorf("create meta : %w", err)
		}
	}

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open tmp file %s: %w", tmp, err)
	}

	// 预分配空间
	if meta.TotalSize > 0 {
		_ = f.Truncate(meta.TotalSize)
	}

	_, err = d.RunSegments(ctx, task, f, meta, concurrency)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	_ = task.RemoveMeta()
	if err = os.Rename(tmp, dest); err != nil {
		return nil, fmt.Errorf("rename %s -> %s: %w", tmp, dest, err)
	}
	if totalSize > 0 {
		return &Result{Path: dest, Size: totalSize}, nil
	}
	info, statErr := os.Stat(dest)
	if statErr != nil {
		return nil, fmt.Errorf("stat %s: %w", dest, statErr)
	}
	return &Result{Path: dest, Size: info.Size()}, nil
}

func (d *BaseHTTPDownloader) RunSegments(ctx context.Context, task *Task, f *os.File, meta *Meta, concurrency int) (int64, error) {
	var (
		wg sync.WaitGroup

		firstErr atomic.Value
	)
	cmds := make(chan WriteCmd)
	segments := make(chan Segment)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	saveTicker := time.NewTicker(time.Second)
	defer saveTicker.Stop()
	dirtyMeta := false
	flushMeta := func() error {
		if !dirtyMeta {
			return nil
		}
		if err := task.SaveMeta(meta); err != nil {
			return fmt.Errorf("save meta: %w", err)
		}
		dirtyMeta = false
		return nil
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.produceSegments(ctx, task, cmds, segments); err != nil {
				firstErr.CompareAndSwap(nil, err)
				cancel()
			}
		}()
	}

	go func() {
		wg.Wait()
		close(cmds)
	}()

	go func() {
		for _, seg := range meta.Segments {
			if seg.Done {
				continue
			}
			select {
			case segments <- seg:
			case <-ctx.Done():
				return // 生产者都退了就停止发送
			}
		}
		close(segments)
	}()

	var downloaded int64
	for i := range meta.Segments {
		downloaded += meta.Segments[i].Written
	}
	if task.OnProgress != nil {
		task.OnProgress(downloaded, meta.TotalSize)
	}

	var written int64
	for cmd := range cmds {
		nw, err := f.WriteAt(cmd.buf, cmd.offset)
		if err != nil {
			return written, fmt.Errorf("write segment %d: %w", cmd.segIdx, err)
		}
		written += int64(nw)

		seg := &meta.Segments[cmd.segIdx]
		seg.Written += int64(nw)
		downloaded += int64(nw)
		if seg.End >= 0 && seg.Written >= seg.End-seg.Start+1 {
			seg.Done = true
		}
		dirtyMeta = true
		select {
		case <-saveTicker.C:
			if err := flushMeta(); err != nil {
				return written, err
			}
		default:
		}
		if task.OnProgress != nil {
			task.OnProgress(downloaded, meta.TotalSize)
		}
	}

	if err := flushMeta(); err != nil {
		return written, err
	}

	if v := firstErr.Load(); v != nil {
		return 0, v.(error)
	}
	return written, nil

}

func (d *BaseHTTPDownloader) produceSegments(ctx context.Context, task *Task, cmds chan<- WriteCmd, segments <-chan Segment) error {
	segRequest := task.Request.Clone(ctx)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case seg, ok := <-segments:
			if !ok {
				return nil
			}
			for attempt := 0; attempt <= task.Retry; attempt++ {
				err := d.downloadSegment(ctx, task, segRequest, &seg, cmds)
				if err != nil {
					if attempt == task.Retry {
						return err
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(task.Interval()):
					}
				} else {
					break
				}
			}
		}

	}
}

func (d *BaseHTTPDownloader) downloadSegment(ctx context.Context, task *Task, segRequest *http.Request, seg *Segment, cmds chan<- WriteCmd) error {

	resumeOffset := seg.Start + seg.Written
	if seg.End >= 0 {
		segRequest.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", resumeOffset, seg.End))
	} else if seg.Written > 0 {
		segRequest.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	} else {
		segRequest.Header.Del("Range")
	}

	resp, err := d.requester.Request(ctx, segRequest, task.Opts)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code %d for segment [%d-%d]", resp.StatusCode, seg.Start, seg.End)
	}

	if seg.End >= 0 && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("segment %d-%d: expected 206, got %d", seg.Start, seg.End, resp.StatusCode)
	}

	offset := resumeOffset
	buf := make([]byte, defaultChunkSize)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			payload := make([]byte, n)
			copy(payload, buf[:n])
			select {
			case cmds <- WriteCmd{segIdx: seg.Idx, offset: offset, buf: payload}:
			case <-ctx.Done():
				return ctx.Err()
			}
			// Move resume cursor immediately so retries continue from the latest enqueued byte.
			seg.Written += int64(n)
			offset += int64(n)
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read segment %d: %w", seg.Start, readErr)
		}
	}
}

func (d *BaseHTTPDownloader) Fetch(ctx context.Context, task *Task) (*http.Response, error) {
	if task == nil || task.Request == nil {
		return nil, fmt.Errorf("task or request is nil")
	}
	opts := ensureTaskOpts(task)

	for attempt := 0; attempt <= task.Retry; attempt++ {
		resp, err := d.requester.Request(ctx, task.Request, opts)
		if err != nil {
			if attempt == task.Retry {
				return nil, err
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(task.Interval()):
			}
		} else {
			return resp, nil
		}
	}
	return nil, fmt.Errorf("failed to fetch URL after %d attempts", task.Retry+1)
}

func patchPath(task *Task, h http.Header) error {
	if task.SavePath != "" {
		return nil
	}

	dest := strings.TrimSpace(task.Dest)
	if dest == "" {
		dest = "."
	}

	urlName := FilenameFromURL(task.Request.URL.String())
	fileName := ""

	if h != nil {
		fileName = FilenameFromHeader(h)
	}
	if fileName == "" {
		fileName = urlName
	}
	if filepath.Ext(fileName) == "" && h != nil {
		if ext := ExtFromContentType(h.Get("Content-Type")); ext != "" {
			fileName += ext
		}
	}
	if fileName == "" {
		fileName = uuid.NewString()
	}

	dest = filepath.Clean(dest)
	if info, err := os.Stat(dest); err == nil {
		if info.IsDir() {
			task.SavePath = filepath.Join(dest, fileName)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		task.SavePath = dest
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if IsDirPath(dest) {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		task.SavePath = filepath.Join(dest, fileName)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	task.SavePath = dest
	return nil
}
