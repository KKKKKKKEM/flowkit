package http_download

import (
	"sync"
	"time"

	"github.com/vbauerster/mpb/v8"
)

var (
	sharedProgressOnce sync.Once
	sharedProgress     *mpb.Progress
)

type stageOptions struct {
	progressBar   bool
	proxy         string
	timeout       time.Duration
	retry         int
	retryInterval time.Duration
	inputKey      string // 从 rc.Inputs 中读取 Task 的 key，默认为 "task"
	nextStageName string
	headers       map[string]string
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

func WithInputKey(inputKey string) Option {
	return func(o *stageOptions) {
		o.inputKey = inputKey
	}
}

func WithNextStage(stageName string) Option {
	return func(o *stageOptions) {
		o.nextStageName = stageName
	}
}

func WithHeaders(headers map[string]string) Option {
	return func(o *stageOptions) { o.headers = headers }
}
func WithHeader(key, value string) Option {
	return func(o *stageOptions) {
		if o.headers == nil {
			o.headers = make(map[string]string)
		}
		o.headers[key] = value
	}
}

func getSharedProgress() *mpb.Progress {
	sharedProgressOnce.Do(func() {
		sharedProgress = mpb.New(
			mpb.WithRefreshRate(120 * time.Millisecond),
		)
	})
	return sharedProgress
}
