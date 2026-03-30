package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/KKKKKKKEM/flowkit/stages/download"
	"github.com/KKKKKKKEM/flowkit/stages/download/http/fetcher"
	"github.com/google/uuid"
)

type Requester interface {
	Name() string
	Request(ctx context.Context, req *http.Request, opts *download.Opts) (*http.Response, error)
}

const defaultChunkSize int64 = 1 * 1024 * 1024

type Downloader struct {
	requester Requester
}

func NewHTTPDownloader() *Downloader {
	return &Downloader{requester: &fetcher.HttpClient{}}
}

func (d *Downloader) Name() string { return "http" }

func (d *Downloader) CanHandle(task *download.Task) bool {
	if task == nil || task.URI == "" {
		return false
	}
	scheme, _, ok := strings.Cut(task.URI, "://")
	if !ok {
		return false
	}
	return scheme == "http" || scheme == "https"
}

func (d *Downloader) buildRequest(ctx context.Context, task *download.Task) (*http.Request, error) {
	method := http.MethodGet
	if m, ok := task.Meta["method"].(string); ok && m != "" {
		method = strings.ToUpper(m)
	}

	var body io.Reader
	if b, ok := task.Meta["body"].(io.Reader); ok {
		body = b
	}

	req, err := http.NewRequestWithContext(ctx, method, task.URI, body)
	if err != nil {
		return nil, err
	}

	if headers, ok := task.Meta["headers"].(map[string]string); ok {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	return req, nil
}

type writeCmd struct {
	segIdx int
	offset int64
	buf    []byte
}

func (d *Downloader) probe(ctx context.Context, task *download.Task) (int64, bool, http.Header) {
	req, err := d.buildRequest(ctx, task)
	if err != nil {
		return -1, false, nil
	}
	req.Method = http.MethodHead

	resp, err := d.requester.Request(ctx, req, task.Opts)
	if err != nil {
		return -1, false, nil
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return -1, false, nil
	}

	acceptsRanges := strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes")
	return resp.ContentLength, acceptsRanges, resp.Header
}

func (d *Downloader) buildSegments(totalSize, chunkSize int64, concurrency int) []download.Segment {
	if totalSize > 0 && concurrency > 1 {
		var segments []download.Segment
		for start := int64(0); start < totalSize; start += chunkSize {
			end := start + chunkSize - 1
			if end >= totalSize {
				end = totalSize - 1
			}
			segments = append(segments, download.Segment{Start: start, End: end, Idx: int(start / chunkSize)})
		}
		return segments
	}
	return []download.Segment{{Start: 0, End: -1}}
}

func (d *Downloader) Download(ctx context.Context, task *download.Task) (*download.Result, error) {
	if task == nil || task.URI == "" {
		return nil, fmt.Errorf("task or URI is empty")
	}
	if task.Opts == nil {
		task.Opts = &download.Opts{}
	}

	totalSize, acceptsRanges, probeHeader := d.probe(ctx, task)
	if err := patchPath(task, probeHeader); err != nil {
		return nil, err
	}

	dest := task.Opts.SavePath
	tmp := dest + ".tmp"
	chunkSize := task.Opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}

	concurrency := task.Opts.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency <= 1 || !acceptsRanges || totalSize <= 0 {
		concurrency = 1
	}

	allSegments := d.buildSegments(totalSize, chunkSize, concurrency)
	var meta *download.Meta

	switch {
	case task.Opts.Overwrite:
		_ = os.Remove(dest)
		_ = os.Remove(tmp)
		_ = removeMeta(task)

	case download.FileExistsAt(dest):
		if info, err := os.Stat(dest); err == nil {
			return &download.Result{Path: dest, Size: info.Size()}, nil
		}

	case download.FileExistsAt(tmp):
		if m, err := loadMeta(task); err == nil &&
			m.TotalSize == totalSize &&
			m.ChunkSize == chunkSize &&
			len(m.Segments) == len(allSegments) {
			meta = m
		}
	}

	if meta == nil {
		_ = os.Remove(tmp)
		_ = removeMeta(task)
		meta = &download.Meta{TotalSize: totalSize, ChunkSize: chunkSize, Segments: allSegments}
		if err := saveMeta(task, meta); err != nil {
			return nil, fmt.Errorf("create meta: %w", err)
		}
	}

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open tmp file %s: %w", tmp, err)
	}
	if meta.TotalSize > 0 {
		_ = f.Truncate(meta.TotalSize)
	}

	_, err = d.runSegments(ctx, task, f, meta, concurrency)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	_ = removeMeta(task)
	if err = os.Rename(tmp, dest); err != nil {
		return nil, fmt.Errorf("rename %s -> %s: %w", tmp, dest, err)
	}
	if totalSize > 0 {
		return &download.Result{Path: dest, Size: totalSize}, nil
	}
	info, statErr := os.Stat(dest)
	if statErr != nil {
		return nil, fmt.Errorf("stat %s: %w", dest, statErr)
	}
	return &download.Result{Path: dest, Size: info.Size()}, nil
}

func (d *Downloader) runSegments(ctx context.Context, task *download.Task, f *os.File, meta *download.Meta, concurrency int) (int64, error) {
	var (
		wg       sync.WaitGroup
		firstErr error
		errOnce  sync.Once
	)
	cmds := make(chan writeCmd)
	segments := make(chan download.Segment)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	setFirstErr := func(err error) {
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		errOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	saveTicker := time.NewTicker(time.Second)
	defer saveTicker.Stop()
	dirtyMeta := false
	flushMeta := func() error {
		if !dirtyMeta {
			return nil
		}
		if err := saveMeta(task, meta); err != nil {
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
				setFirstErr(err)
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
				return
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
	var writeErr error
	for cmd := range cmds {
		if writeErr != nil {
			continue
		}
		nw, err := f.WriteAt(cmd.buf, cmd.offset)
		if err != nil {
			writeErr = fmt.Errorf("write segment %d: %w", cmd.segIdx, err)
			setFirstErr(writeErr)
			continue
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
				writeErr = err
				setFirstErr(err)
				continue
			}
		default:
		}
		if task.OnProgress != nil {
			task.OnProgress(downloaded, meta.TotalSize)
		}
	}

	if writeErr != nil {
		return written, writeErr
	}
	if err := flushMeta(); err != nil {
		return written, err
	}
	if firstErr != nil {
		return 0, firstErr
	}
	return written, nil
}

func (d *Downloader) produceSegments(ctx context.Context, task *download.Task, cmds chan<- writeCmd, segments <-chan download.Segment) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case seg, ok := <-segments:
			if !ok {
				return nil
			}
			for attempt := 0; attempt <= task.Opts.Retry; attempt++ {
				err := d.downloadSegment(ctx, task, &seg, cmds)
				if err != nil {
					if attempt == task.Opts.Retry {
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

func (d *Downloader) downloadSegment(ctx context.Context, task *download.Task, seg *download.Segment, cmds chan<- writeCmd) error {
	req, err := d.buildRequest(ctx, task)
	if err != nil {
		return err
	}

	resumeOffset := seg.Start + seg.Written
	if seg.End >= 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", resumeOffset, seg.End))
	} else if seg.Written > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
	}

	resp, err := d.requester.Request(ctx, req, task.Opts)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)

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
			case cmds <- writeCmd{segIdx: seg.Idx, offset: offset, buf: payload}:
			case <-ctx.Done():
				return ctx.Err()
			}
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

// ---------------------------------------------------------------------------
// meta 文件辅助（从原 Task 方法移入，基于 Opts.SavePath）
// ---------------------------------------------------------------------------

func metaPath(task *download.Task) (string, error) {
	if task.Opts == nil || task.Opts.SavePath == "" {
		return "", fmt.Errorf("save path not resolved")
	}
	return task.Opts.SavePath + ".meta", nil
}

func loadMeta(task *download.Task) (*download.Meta, error) {
	p, err := metaPath(task)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var m download.Meta
	if err = json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func saveMeta(task *download.Task, m *download.Meta) error {
	p, err := metaPath(task)
	if err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

func removeMeta(task *download.Task) error {
	p, err := metaPath(task)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// ---------------------------------------------------------------------------
// patchPath — 解析最终保存路径（HTTP 版：从 URL / Content-Disposition 推断）
// ---------------------------------------------------------------------------

func patchPath(task *download.Task, h http.Header) error {
	if task.Opts.SavePath != "" {
		return nil
	}

	dest := strings.TrimSpace(task.Opts.Dest)
	if dest == "" {
		dest = "."
	}

	urlName := download.FilenameFromURL(task.URI)
	fileName := ""
	if h != nil {
		fileName = download.FilenameFromHeader(h)
	}
	if fileName == "" {
		fileName = urlName
	}
	if filepath.Ext(fileName) == "" && h != nil {
		if ext := download.ExtFromContentType(h.Get("Content-Type")); ext != "" {
			fileName += ext
		}
	}
	if fileName == "" {
		fileName = uuid.NewString()
	}

	dest = filepath.Clean(dest)
	if info, err := os.Stat(dest); err == nil {
		if info.IsDir() {
			task.Opts.SavePath = filepath.Join(dest, fileName)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		task.Opts.SavePath = dest
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if download.IsDirPath(dest) {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		task.Opts.SavePath = filepath.Join(dest, fileName)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	task.Opts.SavePath = dest
	return nil
}
