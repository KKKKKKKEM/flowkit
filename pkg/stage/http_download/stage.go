package http_download

import (
	"fmt"
	"sync"
	"time"

	"github.com/KKKKKKKEM/grasp/pkg/core"
	"github.com/KKKKKKKEM/grasp/pkg/download"
	"github.com/KKKKKKKEM/grasp/pkg/download/http"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

type DirectDownloadStage struct {
	stageName string // stage 唯一标识符
	opts      stageOptions
}

func (s *DirectDownloadStage) Name() string {
	return s.stageName
}

// applyFallback 将 fb 中的非零值填充到 task.Opts，header 仅补充不覆盖。
func applyFallback(task *download.Task, fb *download.Opts, headers map[string]string) {
	if task.Opts == nil {
		task.Opts = &download.Opts{}
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
	if headers != nil {
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

func (s *DirectDownloadStage) Run(rc *core.RunContext) core.StageResult {
	// 优先从运行时输入读取 Task，其次使用构造时指定的默认 Task
	var task *download.Task

	inputKey := s.opts.inputKey
	if inputKey == "" {
		inputKey = "task"
	}

	if val, ok := rc.Values[inputKey]; ok {
		if t, ok := val.(*download.Task); ok {
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
	applyFallback(task, &o.fallback, o.headers)

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

			if lastBytes == 0 {
				lastBytes = downloaded
				bar.SetCurrent(downloaded)
			} else {
				delta := downloaded - lastBytes
				if delta > 0 {
					bar.EwmaIncrInt64(delta, 120*time.Millisecond)
					lastBytes = downloaded
				}
			}

			if origProgress != nil {
				origProgress(downloaded, total)
			}
		}

		origComplete := task.OnComplete
		task.OnComplete = func(result *download.DownloadResult) {
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
			Next:   o.nextStageName,
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
		Next:   o.nextStageName,
		Outputs: map[string]any{
			"download_result": result,
		},
	}
}

// NewStage 创建一个 DirectDownloadStage
func NewStage(name string, options ...Option) *DirectDownloadStage {
	s := &DirectDownloadStage{
		stageName: name,
	}
	for _, opt := range options {
		opt(&s.opts)
	}
	return s
}
