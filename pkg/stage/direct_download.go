package stage

import (
	"context"
	"fmt"
	"time"

	"github.com/KKKKKKKEM/grasp/pkg/core"
	downloaderImp "github.com/KKKKKKKEM/grasp/pkg/downloader"
	"github.com/KKKKKKKEM/grasp/pkg/downloader/http"
)

type stageOptions struct {
	progressBar   bool
	proxy         string
	timeout       time.Duration
	retry         int
	retryInterval time.Duration
}

type Option func(*stageOptions)

func WithProgressBar() Option {
	return func(o *stageOptions) { o.progressBar = true }
}

func WithProxy(proxyURL string) Option {
	return func(o *stageOptions) { o.proxy = proxyURL }
}

func WithEnvProxy() Option {
	return func(o *stageOptions) { o.proxy = "env" }
}

func WithTimeout(d time.Duration) Option {
	return func(o *stageOptions) { o.timeout = d }
}

func WithRetry(maxAttempts int, interval time.Duration) Option {
	return func(o *stageOptions) {
		o.retry = maxAttempts
		o.retryInterval = interval
	}
}

type DirectDownloadStage struct {
	Task *downloaderImp.Task
	opts stageOptions
}

func (s *DirectDownloadStage) Do(ctx context.Context) (core.Stage, error) {
	task := s.Task
	o := s.opts
	if task == nil {
		return nil, fmt.Errorf("task is nil")
	}
	if task.Opts == nil {
		task.Opts = &downloaderImp.Opts{}
	}

	if task.Proxy == "" && o.proxy != "" {
		task.Proxy = o.proxy
	}
	if task.Timeout == 0 && o.timeout > 0 {
		task.Timeout = o.timeout
	}
	if task.Retry == 0 && o.retry > 0 {
		task.Retry = o.retry
	}
	if task.RetryInterval == 0 && o.retryInterval > 0 {
		task.RetryInterval = o.retryInterval
	}

	if o.progressBar && task.OnProgress == nil {
		origProgress := task.OnProgress
		bar := newProgressBar(-1)
		task.OnProgress = func(downloaded, total int64) {
			if bar.total != total {
				bar.mu.Lock()
				bar.total = total
				bar.mu.Unlock()
			}
			bar.update(downloaded)
			if origProgress != nil {
				origProgress(downloaded, total)
			}
		}
		origComplete := task.OnComplete
		task.OnComplete = func(result *downloaderImp.DownloadResult) {
			bar.finish()
			if origComplete != nil {
				origComplete(result)
			}
		}
	}

	downloader := http.NewSimpleHTTPDownloader()
	_, err := downloader.Download(ctx, task)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func NewDirectDownloadStage(task *downloaderImp.Task, options ...Option) *DirectDownloadStage {
	s := &DirectDownloadStage{Task: task}
	for _, opt := range options {
		opt(&s.opts)
	}
	return s
}
