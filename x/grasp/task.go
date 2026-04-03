package grasp

import (
	"time"

	"github.com/KKKKKKKEM/flowkit/stages/download"
	"github.com/KKKKKKKEM/flowkit/stages/extract"
)

type Task struct {
	URLs    []string          `cli:"url,required,usage=URLs to grasp (repeatable: -url a -url b)"`
	Proxy   string            `cli:"proxy,usage=proxy URL"`
	Timeout time.Duration     `cli:"timeout,default=30s,usage=request timeout"`
	Retry   int               `cli:"retry,default=3,usage=retry count"`
	Headers map[string]string `cli:"header,usage=extra request headers (repeatable: -header Key=Val)"`

	Extract  ExtractConfig  `cli:"extract"`
	Download DownloadConfig `cli:"download"`

	Selector  SelectFunc    `cli:"-"`
	Transform TransformFunc `cli:"-"`
}

type ExtractConfig struct {
	MaxRounds         int    `cli:"rounds,default=1,usage=max extract rounds"`
	ForcedParser      string `cli:"parser,usage=force a specific parser by name"`
	WorkerConcurrency int    `cli:"concurrency,default=1,usage=max concurrent extract workers"`
}

type DownloadConfig struct {
	Dest               string        `cli:"dest,default=.,usage=download destination directory"`
	Overwrite          bool          `cli:"overwrite,usage=overwrite existing files"`
	TaskConcurrency    int           `cli:"concurrency,usage=max concurrent download tasks (0 = no limit)"`
	BestEffort         bool          `cli:"best-effort,usage=continue after individual task failures"`
	SegmentConcurrency int           `cli:"segments,default=3,usage=concurrent segments per download task"`
	ChunkSize          int64         `cli:"chunk-size,usage=chunk size in bytes (0 = auto)"`
	RetryInterval      time.Duration `cli:"retry-interval,usage=interval between retries"`
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
