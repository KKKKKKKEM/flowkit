package download

import (
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

type Stage struct {
	*core.TypedStageAdapter[[]*Task, []*Result]
	stageName   string
	opts        stageOptions
	downloaders []Downloader
}

func (s *Stage) Name() string {
	return s.stageName
}

func (s *Stage) ApplyFallback(task *Task, fb *Opts) {
	if task.Opts == nil {
		task.Opts = &Opts{}
	}
	if task.Opts.Proxy == "" {
		task.Opts.Proxy = fb.Proxy
	}
	if task.Opts.Timeout == 0 {
		task.Opts.Timeout = fb.Timeout
	}
	if task.Opts.Retry == 0 {
		task.Opts.Retry = fb.Retry
	}
	if task.Opts.RetryInterval == 0 {
		task.Opts.RetryInterval = fb.RetryInterval
	}
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

	for _, task := range in {
		s.ApplyFallback(task, &s.opts.fallback)
	}

	result.Output, err = s.download(rc, in)
	return
}

func (s *Stage) download(rc *core.Context, tasks []*Task) ([]*Result, error) {
	resultsCh := make(chan batchResult, len(tasks))
	var wg sync.WaitGroup

	for idx, task := range tasks {
		wg.Add(1)
		go func(i int, t *Task) {
			defer wg.Done()
			dl, err := s.Dispatch(t)
			if err != nil {
				resultsCh <- batchResult{idx: i, uri: t.URI, err: err}
				return
			}
			res, err := dl.Download(rc, t)
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
		if br.err != nil {
			return nil, fmt.Errorf("download failed for %s: %w", br.uri, br.err)
		}
		results[br.idx] = br.result
	}

	return results, nil
}

func NewStage(name string, options ...Option) *Stage {
	s := &Stage{
		stageName: name,
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
