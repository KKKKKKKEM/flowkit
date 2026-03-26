package extract

import (
	"github.com/KKKKKKKEM/grasp/pkg/extract"
)

type stageOptions struct {
	fallback        extract.Opts // proxy/timeout/retry/headers 等兜底值
	inputKey        string       // 从 rc.Values 中读取 Task 的 key，默认为 "task"
	nextStageName   string
	maxRounds       int
	defaultSelector extract.Selector
}

type Option func(*stageOptions)

func WithFallback(opts *extract.Opts) Option {
	return func(o *stageOptions) {
		if opts != nil {
			o.fallback = *opts
		}
	}
}

func WithInputKey(inputKey string) Option {
	return func(o *stageOptions) { o.inputKey = inputKey }
}

func WithNextStage(stageName string) Option {
	return func(o *stageOptions) { o.nextStageName = stageName }
}
