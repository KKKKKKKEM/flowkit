package extractor

import (
	"github.com/KKKKKKKEM/grasp/pkg/extractors"
)

type stageOptions struct {
	fallback        extractors.Opts // proxy/timeout/retry/headers 等兜底值
	inputKey        string          // 从 rc.Values 中读取 Task 的 key，默认为 "task"
	nextStageName   string
	maxRounds       int
	defaultSelector extractors.Selector
}

type Option func(*stageOptions)

func WithFallback(opts *extractors.Opts) Option {
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
