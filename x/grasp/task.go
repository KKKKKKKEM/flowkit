package grasp

import (
	"time"

	"github.com/KKKKKKKEM/flowkit/stages/download"
	"github.com/KKKKKKKEM/flowkit/stages/extract"
)

type Task struct {
	URLs    []string
	Proxy   string
	Timeout time.Duration
	Retry   int
	Headers map[string]string

	Extract  ExtractConfig
	Download DownloadConfig

	Selector  SelectFunc
	Transform TransformFunc
}

type ExtractConfig struct {
	MaxRounds         int
	ForcedParser      string
	WorkerConcurrency int
}

type DownloadConfig struct {
	Dest               string
	Overwrite          bool
	TaskConcurrency    int
	BestEffort         bool
	SegmentConcurrency int
	ChunkSize          int64
	RetryInterval      time.Duration
}

func (t *Task) toExtractOpts() *extract.Opts {
	return &extract.Opts{
		Proxy:   t.Proxy,
		Timeout: t.Timeout,
		Retry:   t.Retry,
		Headers: t.Headers,
	}
}

func (t *Task) toDownloadOpts() *download.Opts {
	return &download.Opts{
		Proxy:         t.Proxy,
		Timeout:       t.Timeout,
		Retry:         t.Retry,
		Dest:          t.Download.Dest,
		Overwrite:     t.Download.Overwrite,
		Concurrency:   t.Download.SegmentConcurrency,
		ChunkSize:     t.Download.ChunkSize,
		RetryInterval: t.Download.RetryInterval,
	}
}

func (t *Task) resolveSelector(pipelineDefault SelectFunc) SelectFunc {
	if t.Selector != nil {
		return t.Selector
	}
	if pipelineDefault != nil {
		return pipelineDefault
	}
	return SelectAll
}

func (t *Task) resolveTransform(pipelineDefault TransformFunc) TransformFunc {
	if t.Transform != nil {
		return t.Transform
	}
	if pipelineDefault != nil {
		return pipelineDefault
	}
	return DefaultTransform(t.toDownloadOpts())
}
