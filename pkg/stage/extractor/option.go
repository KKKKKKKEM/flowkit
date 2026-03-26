package extractor

import (
	"time"

	"github.com/KKKKKKKEM/grasp/pkg/extractors"
)

type stageOptions struct {
	proxy           string
	timeout         time.Duration
	retry           int
	headers         map[string]string
	inputKey        string // 从 rc.Inputs 中读取 Task 的 key，默认为 "task"
	nextStageName   string
	maxRounds       int
	defaultSelector extractors.Selector
}
type Option func(*stageOptions)

func WithInputKey(inputKey string) Option {
	return func(o *stageOptions) {
		o.inputKey = inputKey
	}
}
func WithProxy(proxyURL string) Option {
	return func(o *stageOptions) { o.proxy = proxyURL }
}

func WithEnvProxy() Option {
	return func(o *stageOptions) { o.proxy = "env" }
}
func WithNextStage(stageName string) Option {
	return func(o *stageOptions) {
		o.nextStageName = stageName
	}
}
func WithRetry(maxAttempts int, interval time.Duration) Option {
	return func(o *stageOptions) {
		o.retry = maxAttempts
	}
}
func WithTimeout(d time.Duration) Option {
	return func(o *stageOptions) { o.timeout = d }
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
