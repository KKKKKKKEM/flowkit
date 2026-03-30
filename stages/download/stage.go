package download

import (
	"fmt"
	"sync"

	"github.com/KKKKKKKEM/flowkit/core"
)

type batchResult struct {
	idx    int
	task   *Task
	result *Result
	err    error
}

type Stage struct {
	*core.TypedStageAdapter[[]*Task, []*Result]
	stageName string
	opts      stageOptions
}

func (s *Stage) Name() string {
	return s.stageName
}

func applyFallback(task *Task, fb *Opts, headers map[string]string) {
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
	if task.Opts.Headers == nil && fb.Headers != nil {
		task.Opts.Headers = make(map[string]string, len(fb.Headers))
		for k, v := range fb.Headers {
			task.Opts.Headers[k] = v
		}
	} else if fb.Headers != nil {
		for k, v := range fb.Headers {
			if _, exists := task.Opts.Headers[k]; !exists {
				task.Opts.Headers[k] = v
			}
		}
	}
	if headers != nil {
		if task.Request == nil {
			return
		}
		if task.Request.Header == nil {
			task.Request.Header = make(map[string][]string)
		}
		for k, v := range headers {
			if _, exists := task.Request.Header[k]; !exists {
				task.Request.Header[k] = []string{v}
			}
		}
	}
}

func (s *Stage) Exec(rc *core.Context, in []*Task) (result core.TypedResult[[]*Result], err error) {
	result.Next = s.opts.nextStageName

	for _, task := range in {
		applyFallback(task, &s.opts.fallback, s.opts.fallback.Headers)
	}

	if len(in) == 1 {
		var one *Result
		one, err = s.downloadOne(rc, in[0])
		if err != nil {
			return
		}
		result.Output = []*Result{one}
		return
	}

	result.Output, err = s.downloadBatch(rc, in)
	return
}

func (s *Stage) downloadOne(rc *core.Context, task *Task) (*Result, error) {
	dl := NewHTTPDownloader()
	result, err := dl.Download(rc, task)
	if err != nil {
		return nil, err
	}
	if task.OnComplete != nil {
		task.OnComplete(result)
	}
	return result, nil
}

func (s *Stage) downloadBatch(rc *core.Context, tasks []*Task) ([]*Result, error) {
	resultsCh := make(chan batchResult, len(tasks))
	var wg sync.WaitGroup

	for idx, task := range tasks {
		wg.Add(1)
		go func(i int, t *Task) {
			defer wg.Done()
			dl := NewHTTPDownloader()
			res, err := dl.Download(rc, t)
			if err == nil && t.OnComplete != nil {
				t.OnComplete(res)
			}
			resultsCh <- batchResult{idx: i, task: t, result: res, err: err}
		}(idx, task)
	}

	wg.Wait()
	close(resultsCh)

	results := make([]*Result, len(tasks))
	for br := range resultsCh {
		if br.err != nil {
			return nil, fmt.Errorf("download failed for %s: %w", br.task.Request.URL, br.err)
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
	inputKey := "tasks"
	outputKey := "results"

	s.TypedStageAdapter = core.NewTypedStage[[]*Task, []*Result](
		name,
		inputKey,
		outputKey,
		s,
	)
	return s
}
