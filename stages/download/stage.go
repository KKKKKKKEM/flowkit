package download

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/KKKKKKKEM/flowkit/core"
)

type batchResult struct {
	idx    int
	uri    string
	result *Result
	err    error
}

type BatchItemError struct {
	Index int
	URI   string
	Err   error
}

type BatchError struct {
	Failures []BatchItemError
}

func (e *BatchError) Error() string {
	if e == nil || len(e.Failures) == 0 {
		return "download batch failed"
	}
	first := e.Failures[0]
	return fmt.Sprintf("%d download(s) failed, first failure [%d] %s: %v", len(e.Failures), first.Index, first.URI, first.Err)
}

func (e *BatchError) Unwrap() []error {
	if e == nil {
		return nil
	}
	errList := make([]error, 0, len(e.Failures))
	for _, failure := range e.Failures {
		errList = append(errList, failure.Err)
	}
	return errList
}

type Stage struct {
	*core.TypedStageAdapter[[]*Task, []*Result]
	stageName   string
	opts        stageOptions
	downloaders []Downloader
}

func (s *Stage) Name() string {
	return s.stageName
}

func (s *Stage) resolveTasks(tasks []*Task) []*Task {
	resolved := make([]*Task, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		resolved = append(resolved, task.CloneWithOpts(ResolveOpts(task.Opts, &s.opts.defaults)))
	}
	return resolved
}

// Register 追加一个 Downloader（后注册的优先级更高）。
func (s *Stage) Register(downloaders ...Downloader) {
	for _, downloader := range downloaders {
		s.downloaders = append([]Downloader{downloader}, s.downloaders...)
	}
}

// Dispatch 找到第一个能处理该任务的 Downloader。
func (s *Stage) Dispatch(task *Task) (Downloader, error) {
	for _, d := range s.downloaders {
		if d.CanHandle(task) {
			return d, nil
		}
	}
	return nil, fmt.Errorf("no downloader available for URI: %s", task.URI)
}

func (s *Stage) Exec(rc *core.Context, in []*Task) (result core.TypedResult[[]*Result], err error) {
	result.Next = s.opts.nextStageName
	result.Output, err = s.download(rc, s.resolveTasks(in))
	return
}

func (s *Stage) download(rc *core.Context, tasks []*Task) ([]*Result, error) {
	if len(tasks) == 0 {
		return nil, nil
	}

	batchCtx, cancel := context.WithCancel(rc)
	defer cancel()

	resultsCh := make(chan batchResult, len(tasks))
	var wg sync.WaitGroup
	limit := s.opts.maxConcurrency
	if limit <= 0 || limit > len(tasks) {
		limit = len(tasks)
	}
	sem := make(chan struct{}, limit)

	var (
		failuresMu sync.Mutex
		failures   []BatchItemError
		firstOnce  sync.Once
	)
	recordFailure := func(i int, uri string, err error) {
		if err == nil {
			return
		}
		failuresMu.Lock()
		failures = append(failures, BatchItemError{Index: i, URI: uri, Err: err})
		failuresMu.Unlock()
		firstOnce.Do(func() {
			if s.opts.failStrategy != BatchBestEffort {
				cancel()
			}
		})
	}

	for idx, task := range tasks {
		select {
		case <-batchCtx.Done():
			continue
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(i int, t *Task) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := batchCtx.Err(); err != nil {
				return
			}
			dl, err := s.Dispatch(t)
			if err != nil {
				if t.OnError != nil {
					t.OnError(err)
				}
				recordFailure(i, t.URI, err)
				resultsCh <- batchResult{idx: i, uri: t.URI, err: err}
				return
			}
			res, err := dl.Download(batchCtx, t)
			if err != nil {
				if t.OnError != nil {
					t.OnError(err)
				}
				recordFailure(i, t.URI, err)
			}
			if err == nil && t.OnComplete != nil {
				t.OnComplete(res)
			}
			resultsCh <- batchResult{idx: i, uri: t.URI, result: res, err: err}
		}(idx, task)
	}

	wg.Wait()
	close(resultsCh)

	results := make([]*Result, len(tasks))
	for br := range resultsCh {
		if br.err == nil {
			results[br.idx] = br.result
		}
	}

	if len(failures) > 0 {
		if s.opts.failStrategy == BatchBestEffort {
			return results, &BatchError{Failures: failures}
		}
		return nil, &BatchError{Failures: failures}
	}
	if err := batchCtx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return nil, err
	}

	return results, nil
}

func NewStage(name string, options ...Option) *Stage {
	s := &Stage{
		stageName: name,
		opts: stageOptions{
			failStrategy: BatchFailFast,
		},
	}
	for _, opt := range options {
		opt(&s.opts)
	}

	s.Register(s.opts.extraDownloaders...)

	s.TypedStageAdapter = core.NewTypedStage[[]*Task, []*Result](
		name,
		"tasks",
		"results",
		s,
	)
	return s
}

func (s *Stage) Downloaders() []Downloader {
	out := make([]Downloader, len(s.downloaders))
	copy(out, s.downloaders)
	return out
}
