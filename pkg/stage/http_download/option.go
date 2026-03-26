package http_download

import (
	"sync"
	"time"

	"github.com/KKKKKKKEM/grasp/pkg/downloader"
	"github.com/vbauerster/mpb/v8"
)

var (
	sharedProgressOnce sync.Once
	sharedProgress     *mpb.Progress
)

type stageOptions struct {
	fallback      downloader.Opts   // proxy/timeout/retry 等兜底值
	headers       map[string]string // request header 兜底值（downloader.Opts 不含 headers）
	progressBar   bool
	inputKey      string // 从 rc.Values 中读取 Task 的 key，默认为 "task"
	nextStageName string
}

type Option func(*stageOptions)

func WithProgressBar() Option {
	return func(o *stageOptions) { o.progressBar = true }
}

func WithFallback(opts *downloader.Opts) Option {
	return func(o *stageOptions) {
		if opts != nil {
			o.fallback = *opts
		}
	}
}

func WithFallbackHeaders(headers map[string]string) Option {
	return func(o *stageOptions) { o.headers = headers }
}

func WithInputKey(inputKey string) Option {
	return func(o *stageOptions) { o.inputKey = inputKey }
}

func WithNextStage(stageName string) Option {
	return func(o *stageOptions) { o.nextStageName = stageName }
}

func getSharedProgress() *mpb.Progress {
	sharedProgressOnce.Do(func() {
		sharedProgress = mpb.New(
			mpb.WithRefreshRate(120 * time.Millisecond),
		)
	})
	return sharedProgress
}
