package grasp

import (
	"sync"
	"time"

	"github.com/KKKKKKKEM/flowkit/builtin/download"
	"github.com/KKKKKKKEM/flowkit/builtin/serve"
	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

var _ core.ProgressReporter = (*MpbReporter)(nil)
var _ core.ProgressReporter = (*SSEReporter)(nil)

type MpbReporter struct {
	p *mpb.Progress
}

func NewMpbReporter() *MpbReporter {
	return &MpbReporter{
		p: mpb.New(mpb.WithRefreshRate(120 * time.Millisecond)),
	}
}

func (r *MpbReporter) Track(key string, total int64) core.ProgressTracker {
	bar := r.p.AddBar(total,
		mpb.PrependDecorators(
			decor.Name(key+" ", decor.WCSyncWidth),
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
	return &mpbTracker{bar: bar}
}

func (r *MpbReporter) Wait() {
	r.p.Wait()
}

type mpbTracker struct {
	mu        sync.Mutex
	bar       *mpb.Bar
	lastBytes int64
}

func (t *mpbTracker) Update(downloaded int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.lastBytes == 0 {
		t.bar.SetCurrent(downloaded)
	} else if delta := downloaded - t.lastBytes; delta > 0 {
		t.bar.EwmaIncrInt64(delta, 120*time.Millisecond)
	}
	t.lastBytes = downloaded
}

func (t *mpbTracker) Done() {
	t.mu.Lock()
	t.bar.SetTotal(-1, true)
	t.mu.Unlock()
}

type DownloadProgressData struct {
	Key        string `json:"key"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
}

type DownloadDoneData struct {
	Key string `json:"key"`
}

type SSEReporter struct {
	sess *serve.SSESession
}

func NewSSEReporter(sess *serve.SSESession) *SSEReporter {
	return &SSEReporter{sess: sess}
}

func (r *SSEReporter) Track(key string, total int64) core.ProgressTracker {
	r.sess.EmitProgress(DownloadProgressData{Key: key, Downloaded: 0, Total: total})
	return &sseTracker{sess: r.sess, key: key}
}

func (r *SSEReporter) Wait() {}

type sseTracker struct {
	sess *serve.SSESession
	key  string
}

func (t *sseTracker) Update(downloaded int64) {
	t.sess.EmitProgress(DownloadProgressData{Key: t.key, Downloaded: downloaded})
}

func (t *sseTracker) Done() {
	t.sess.EmitProgress(DownloadDoneData{Key: t.key})
}

func bridgeDownloadTask(task *download.Task, tracker core.ProgressTracker) {
	origProgress := task.OnProgress
	task.OnProgress = func(downloaded, total int64) {
		tracker.Update(downloaded)
		if origProgress != nil {
			origProgress(downloaded, total)
		}
	}

	origComplete := task.OnComplete
	task.OnComplete = func(result *download.Result) {
		tracker.Done()
		if origComplete != nil {
			origComplete(result)
		}
	}
}
