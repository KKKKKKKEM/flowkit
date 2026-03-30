package grasp

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/KKKKKKKEM/flowkit"
	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/KKKKKKKEM/flowkit/pipeline"
	"github.com/KKKKKKKEM/flowkit/stages/download"
	dlhttp "github.com/KKKKKKKEM/flowkit/stages/download/http"
	"github.com/KKKKKKKEM/flowkit/stages/extract"
	"github.com/KKKKKKKEM/flowkit/x/grasp/sites/pexels"
	"github.com/google/uuid"
)

type Report struct {
	Success           bool               `json:"success"`
	Partial           bool               `json:"partial,omitempty"`
	DurationMs        int64              `json:"duration_ms"`
	Rounds            int                `json:"rounds"`
	ParsedItems       int                `json:"parsed_items"`
	Downloaded        []*download.Result `json:"downloaded"`
	DownloadSucceeded int                `json:"download_succeeded,omitempty"`
	DownloadFailed    int                `json:"download_failed,omitempty"`
	DownloadFailures  []DownloadFailure  `json:"download_failures,omitempty"`
}

type DownloadFailure struct {
	Index int    `json:"index"`
	URI   string `json:"uri"`
	Error string `json:"error"`
}

type Pipeline struct {
	flowkit.App[*Task, *Report]
	*pipeline.LinearPipeline
	extractor         *extract.Stage
	downloader        *download.Stage
	defaultSelector   SelectFunc
	defaultTransform  TransformFunc
	interactionPlugin core.InteractionPlugin
	trackerProvider   core.TrackerProvider
}

var _ core.Pipeline = (*Pipeline)(nil)

func NewGraspPipeline(opts ...Option) *Pipeline {
	extractor := extract.NewStage("extractor")
	extractor.Mount(&pexels.APIParser{})

	downloader := download.NewStage("download",
		download.WithDownloaders(
			dlhttp.NewHTTPDownloader(),
		),
	)
	p := &Pipeline{
		LinearPipeline:    pipeline.NewLinearPipeline(),
		extractor:         extractor,
		downloader:        downloader,
		trackerProvider:   NewMPBTrackerProvider(),
		interactionPlugin: &CLIInteractionPlugin{},
	}
	for _, opt := range opts {
		opt(p)
	}
	p.App = flowkit.NewApp(p.Invoke)
	return p
}

func (p *Pipeline) CLI(opts ...flowkit.CLIOption[*Task, *Report]) error {
	return p.App.CLI(append([]flowkit.CLIOption[*Task, *Report]{
		flowkit.WithCLIBuilder[*Task, *Report](buildCLI),
		flowkit.WithTrackerProvider[*Task, *Report](p.trackerProvider),
		flowkit.WithInteractionPlugin[*Task, *Report](p.interactionPlugin),
	}, opts...)...)
}

func (p *Pipeline) Serve(addr string, opts ...flowkit.ServeOption[*Task, *Report]) error {
	return p.App.Serve(addr, opts...)
}

func (p *Pipeline) Launch(opts ...flowkit.LaunchOption[*Task, *Report]) error {
	return p.App.Launch(append([]flowkit.LaunchOption[*Task, *Report]{
		flowkit.WithLaunchCLIOptions[*Task, *Report](
			flowkit.WithCLIBuilder[*Task, *Report](buildCLI),
			flowkit.WithTrackerProvider[*Task, *Report](p.trackerProvider),
			flowkit.WithInteractionPlugin[*Task, *Report](p.interactionPlugin),
		),
	}, opts...)...)
}

func (p *Pipeline) Run(rc *core.Context, _ string) (*core.Report, error) {
	task, ok := core.GetState[*Task](rc, "task")
	if !ok {
		err := fmt.Errorf("task missing or wrong type")
		report := &core.Report{
			Mode:         core.ModeLinear,
			TraceID:      rc.Runtime.TraceID,
			StageOrder:   []string{"grasp"},
			StageResults: map[string]core.StageResult{"grasp": {Status: core.StageFailed, Err: err}},
		}
		return report, err
	}
	report := &core.Report{
		Mode:         core.ModeLinear,
		TraceID:      rc.Runtime.TraceID,
		StageOrder:   []string{"grasp"},
		StageResults: make(map[string]core.StageResult),
	}
	start := time.Now()

	graspReport, err := p.Invoke(rc, task)
	if err != nil {
		if graspReport != nil {
			report.StageResults["grasp"] = core.StageResult{
				Status:  core.StageFailed,
				Err:     err,
				Outputs: map[string]any{"report": graspReport},
			}
			report.Success = false
			report.DurationMs = time.Since(start).Milliseconds()
			return report, err
		}
		return fail(report, start, err)
	}

	report.StageResults["grasp"] = core.StageResult{
		Status:  core.StageSuccess,
		Outputs: map[string]any{"report": graspReport},
	}
	report.Success = true
	report.DurationMs = time.Since(start).Milliseconds()
	return report, nil
}

func (p *Pipeline) Invoke(rc *core.Context, task *Task) (*Report, error) {
	return p.run(rc, task)
}

func fail(report *core.Report, start time.Time, err error) (*core.Report, error) {
	report.StageResults["grasp"] = core.StageResult{
		Status: core.StageFailed,
		Err:    err,
	}
	report.Success = false
	report.DurationMs = time.Since(start).Milliseconds()
	return report, err
}

func (p *Pipeline) run(rc *core.Context, task *Task) (*Report, error) {
	start := time.Now()
	report := &Report{}

	allDirect, rounds, err := p.runExtract(rc, task)
	if err != nil {
		return nil, fmt.Errorf("extract: %w", err)
	}
	report.Rounds = rounds
	report.ParsedItems = len(allDirect)

	selected, err := p.selectItems(rc, task, allDirect)
	if err != nil {
		return nil, fmt.Errorf("select: %w", err)
	}

	dlTasks, err := p.buildDownloadTasks(rc, selected, task)
	if err != nil {
		return nil, fmt.Errorf("transform: %w", err)
	}

	results, err := p.runDownload(rc, task, dlTasks)
	report.Downloaded = compactDownloadResults(results)
	report.DownloadSucceeded = len(report.Downloaded)
	if err != nil {
		var batchErr *download.BatchError
		if errors.As(err, &batchErr) {
			report.DownloadFailures = mapDownloadFailures(batchErr)
			report.DownloadFailed = len(report.DownloadFailures)
			report.Partial = report.DownloadSucceeded > 0
			report.Success = task.Download.BestEffort && report.DownloadSucceeded > 0
			report.DurationMs = time.Since(start).Milliseconds()
			if task.Download.BestEffort {
				return report, nil
			}
			return report, fmt.Errorf("download: %w", err)
		}
		return report, fmt.Errorf("download: %w", err)
	}

	report.Success = true
	report.DurationMs = time.Since(start).Milliseconds()
	return report, nil
}

func compactDownloadResults(results []*download.Result) []*download.Result {
	if len(results) == 0 {
		return nil
	}
	filtered := make([]*download.Result, 0, len(results))
	for _, result := range results {
		if result != nil {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func mapDownloadFailures(batchErr *download.BatchError) []DownloadFailure {
	if batchErr == nil || len(batchErr.Failures) == 0 {
		return nil
	}
	failures := make([]DownloadFailure, 0, len(batchErr.Failures))
	for _, failure := range batchErr.Failures {
		failures = append(failures, DownloadFailure{
			Index: failure.Index,
			URI:   failure.URI,
			Error: failure.Err.Error(),
		})
	}
	return failures
}

func (p *Pipeline) selectItems(rc *core.Context, task *Task, items []extract.Item) ([]extract.Item, error) {
	if task.Selector != nil {
		return task.Selector(rc, items)
	}

	i := core.Interaction{Type: core.InteractionTypeSelect, Payload: items, Message: "Please select items to download"}
	interactionPlugin := rc.Runtime.InteractionPlugin
	if interactionPlugin == nil {
		interactionPlugin = p.interactionPlugin
	}

	var indices []int
	if interactionPlugin != nil {
		result, err := interactionPlugin.Interact(rc, i)
		if err != nil {
			return nil, err
		}

		result, err = interactionPlugin.FormatResult(rc, i, result)
		if err != nil {
			return nil, err
		}
		indices, err = toIntSlice(result.Answer)
		if err != nil {
			return nil, fmt.Errorf("select interaction: invalid answer: %w", err)
		}
	} else {
		return task.resolveSelector(p.defaultSelector)(rc, items)
	}

	if len(indices) == 0 {
		return nil, fmt.Errorf("no items selected")
	}

	var selected []extract.Item

	for _, index := range indices {
		selected = append(selected, items[index])
	}

	return selected, nil

}

func toIntSlice(v any) ([]int, error) {
	switch val := v.(type) {
	case []int:
		return val, nil
	case []any:
		out := make([]int, 0, len(val))
		for _, item := range val {
			f, ok := item.(float64)
			if !ok {
				return nil, fmt.Errorf("expected number, got %T", item)
			}
			out = append(out, int(f))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected []int or []any, got %T", v)
	}
}

func (p *Pipeline) runExtract(rc *core.Context, task *Task) ([]extract.Item, int, error) {
	maxRounds := task.Extract.MaxRounds
	if maxRounds <= 0 {
		maxRounds = 1
	}
	extractConcurrency := task.Extract.WorkerConcurrency
	if extractConcurrency <= 0 {
		extractConcurrency = 1
	}

	extractOpts := task.toExtractOpts()
	var allDirect []extract.Item
	queue := make([]string, len(task.URLs))
	copy(queue, task.URLs)

	for round := 0; round < maxRounds && len(queue) > 0; round++ {
		items, next, err := p.extractRound(rc, queue, task.Extract.ForcedParser, extractOpts, extractConcurrency)
		if err != nil {
			return nil, round + 1, err
		}
		allDirect = append(allDirect, items...)
		queue = next
		if round == 0 {
			task.Extract.ForcedParser = ""
		}
	}

	return allDirect, maxRounds, nil
}

func (p *Pipeline) extractRound(
	rc *core.Context,
	urls []string,
	forcedParser string,
	opts *extract.Opts,
	concurrency int,
) (direct []extract.Item, nextQueue []string, err error) {
	type result struct {
		items []extract.Item
		err   error
	}

	sem := make(chan struct{}, concurrency)
	results := make([]result, len(urls))
	var wg sync.WaitGroup

	for i, url := range urls {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, u string) {
			defer wg.Done()
			defer func() { <-sem }()

			child := rc.Fork(uuid.NewString())
			baseTask := &extract.Task{
				URL:          u,
				Opts:         opts,
				ForcedParser: forcedParser,
			}
			if opts != nil {
				baseTask = baseTask.CloneWithURL(u)
				baseTask.ForcedParser = forcedParser
			}
			typed, err := p.extractor.Exec(child, baseTask)
			if err != nil {
				results[idx] = result{err: err}
				return
			}

			results[idx] = result{items: typed.Output}

		}(i, url)
	}
	wg.Wait()

	for _, r := range results {
		if r.err != nil {
			return nil, nil, r.err
		}
		for _, item := range r.items {
			if item.IsDirect {
				direct = append(direct, item)
			} else {
				nextQueue = append(nextQueue, item.URI)
			}
		}
	}
	return direct, nextQueue, nil
}

func (p *Pipeline) buildDownloadTasks(rc *core.Context, items []extract.Item, task *Task) ([]*download.Task, error) {
	transformFn := task.resolveTransform(p.defaultTransform)
	baseOpts := task.toDownloadOpts()

	trackerBuilder := rc.Runtime.TrackerProvider
	if trackerBuilder == nil {
		trackerBuilder = p.trackerProvider
	}

	tasks := make([]*download.Task, 0, len(items))
	for _, item := range items {
		t, err := transformFn(rc, item, baseOpts)
		if err != nil {
			return nil, fmt.Errorf("transform %q: %w", item.URI, err)
		}
		if trackerBuilder != nil {
			key := item.URI
			if item.Name != "" {
				key = item.Name
			}
			tracker := trackerBuilder.Track(key, map[string]any{"total": 0})
			bridgeDownloadTask(t, tracker)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (p *Pipeline) runDownload(rc *core.Context, task *Task, tasks []*download.Task) ([]*download.Result, error) {
	child := rc.Fork(uuid.NewString())
	options := []download.Option{
		download.WithDownloaders(p.downloader.Downloaders()...),
		download.WithDefaults(task.toDownloadOpts()),
		download.WithMaxConcurrency(task.Download.TaskConcurrency),
	}
	if task.Download.BestEffort {
		options = append(options, download.WithFailStrategy(download.BatchBestEffort))
	}
	runner := download.NewStage(p.downloader.Name(), options...)
	typed, err := runner.Exec(child, tasks)
	if err != nil {
		return nil, err
	}
	return typed.Output, nil
}

func bridgeDownloadTask(task *download.Task, tracker core.Tracker) {
	origProgress := task.OnProgress
	task.OnProgress = func(downloaded, total int64) {
		tracker.Update(map[string]any{"current": downloaded, "total": total})
		tracker.Flush()
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
