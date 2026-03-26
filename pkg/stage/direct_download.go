package stage

import (
	"fmt"
	"sync"
	"time"

	"github.com/KKKKKKKEM/grasp/pkg/core"
	"github.com/KKKKKKKEM/grasp/pkg/downloader"
	"github.com/KKKKKKKEM/grasp/pkg/downloader/http"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

var (
	sharedProgressOnce sync.Once
	sharedProgress     *mpb.Progress
)

func getSharedProgress() *mpb.Progress {
	sharedProgressOnce.Do(func() {
		sharedProgress = mpb.New(
			mpb.WithRefreshRate(120 * time.Millisecond),
		)
	})
	return sharedProgress
}

type stageOptions struct {
	progressBar   bool
	proxy         string
	timeout       time.Duration
	retry         int
	retryInterval time.Duration
	inputKey      string // 从 rc.Inputs 中读取 Task 的 key，默认为 "task"
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

type DirectDownloadStage struct {
	stageName string // stage 唯一标识符
	opts      stageOptions
}

func (s *DirectDownloadStage) Name() string {
	return s.stageName
}

func (s *DirectDownloadStage) Run(rc *core.RunContext) core.StageResult {
	// 优先从运行时输入读取 Task，其次使用构造时指定的默认 Task
	var task *downloader.Task

	inputKey := s.opts.inputKey
	if inputKey == "" {
		inputKey = "task"
	}

	if val, ok := rc.Inputs[inputKey]; ok {
		if t, ok := val.(*downloader.Task); ok {
			task = t
		}
	}

	if task == nil {
		return core.StageResult{
			Status: core.StageFailed,
			Err:    fmt.Errorf("task not found: neither in rc.Inputs[\"%s\"] nor in stage default", inputKey),
		}
	}

	o := s.opts
	if task.Opts == nil {
		task.Opts = &downloader.Opts{}
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

	if o.progressBar {
		p := getSharedProgress()
		savePath, err := task.GetSavePath()
		if err != nil {
			return core.StageResult{
				Status: core.StageFailed,
				Err:    fmt.Errorf("failed to get save path: %w", err),
			}
		}

		bar := p.AddBar(0,
			mpb.PrependDecorators(
				decor.Name(savePath+" ", decor.WCSyncWidth),
				decor.Counters(decor.SizeB1024(0), "% .2f / % .2f"),
			),
			mpb.AppendDecorators(
				decor.OnComplete(
					decor.EwmaETA(decor.ET_STYLE_GO, 30, decor.WCSyncWidth),
					"done",
				),
				decor.Name(" "),
				decor.EwmaSpeed(decor.SizeB1024(0), "% .2f", 30, decor.WCSyncWidth),
			),
		)

		var (
			mu         sync.Mutex
			knownTotal int64
			lastBytes  int64
		)

		origProgress := task.OnProgress
		task.OnProgress = func(downloaded, total int64) {
			mu.Lock()
			defer mu.Unlock()

			if total > 0 && total != knownTotal {
				knownTotal = total
				bar.SetTotal(total, false)
			}

			delta := downloaded - lastBytes
			if delta > 0 {
				bar.EwmaIncrInt64(delta, 120*time.Millisecond)
				lastBytes = downloaded
			}

			if origProgress != nil {
				origProgress(downloaded, total)
			}
		}

		origComplete := task.OnComplete
		task.OnComplete = func(result *downloader.DownloadResult) {
			mu.Lock()
			bar.SetTotal(-1, true)
			mu.Unlock()

			if origComplete != nil {
				origComplete(result)
			}
		}

		dl := http.NewSimpleHTTPDownloader()
		result, err := dl.Download(rc, task)
		if err != nil {
			bar.Abort(true)
			p.Wait()
			return core.StageResult{
				Status: core.StageFailed,
				Err:    err,
			}
		}
		if task.OnComplete != nil {
			task.OnComplete(result)
		}

		p.Wait()
		return core.StageResult{
			Status: core.StageSuccess,
			Outputs: map[string]any{
				"download_result": result,
			},
		}
	}

	dl := http.NewSimpleHTTPDownloader()
	result, err := dl.Download(rc, task)
	if err != nil {
		return core.StageResult{
			Status: core.StageFailed,
			Err:    err,
		}
	}
	return core.StageResult{
		Status: core.StageSuccess,
		Outputs: map[string]any{
			"download_result": result,
		},
	}
}

// NewDirectDownloadStage 创建一个 DirectDownloadStage，可选地指定默认 Task
func NewDirectDownloadStage(name string, options ...Option) *DirectDownloadStage {
	s := &DirectDownloadStage{
		stageName: name,
	}
	for _, opt := range options {
		opt(&s.opts)
	}
	return s
}
