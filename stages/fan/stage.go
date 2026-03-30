package fan

import (
	"context"
	"fmt"
	"sync"

	"github.com/KKKKKKKEM/flowkit/core"
)

type Stage struct {
	name     string
	children []core.Stage
	next     string
	opts     options
}

func New(name string, next string, children []core.Stage, opts ...Option) *Stage {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return &Stage{
		name:     name,
		children: children,
		next:     next,
		opts:     o,
	}
}

func (s *Stage) Name() string { return s.name }

func (s *Stage) Run(rc *core.Context) core.StageResult {
	switch {
	case s.opts.wait == WaitAny:
		return s.runWaitAny(rc)
	case s.opts.fail == BestEffort:
		return s.runBestEffort(rc)
	default:
		return s.runWaitAllFailFast(rc)
	}
}

func (s *Stage) runWaitAllFailFast(rc *core.Context) core.StageResult {
	type subResult struct {
		name   string
		result core.StageResult
	}

	ctx, cancel := context.WithCancel(rc)
	defer cancel()

	ch := make(chan subResult, len(s.children))
	var wg sync.WaitGroup

	for _, child := range s.children {
		wg.Add(1)
		go func(child core.Stage) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
			}
			r := child.Run(rc.WithContext(ctx))
			select {
			case ch <- subResult{name: child.Name(), result: r}:
			case <-ctx.Done():
			}
		}(child)
	}

	go func() { wg.Wait(); close(ch) }()

	merged := make(map[string]any)
	for sr := range ch {
		if sr.result.IsFailed() {
			cancel()
			return core.StageResult{
				Status: core.StageFailed,
				Err:    fmt.Errorf("sub-stage %q failed: %w", sr.name, sr.result.Err),
			}
		}
		if err := mergeOutputs(merged, sr.result.Outputs, s.opts.conflict); err != nil {
			cancel()
			return core.StageResult{Status: core.StageFailed, Err: err}
		}
	}

	return core.StageResult{Status: core.StageSuccess, Next: s.next, Outputs: merged}
}

func (s *Stage) runWaitAny(rc *core.Context) core.StageResult {
	type subResult struct {
		name   string
		result core.StageResult
	}

	ctx, cancel := context.WithCancel(rc)
	defer cancel()

	ch := make(chan subResult, len(s.children))
	var wg sync.WaitGroup

	for _, child := range s.children {
		wg.Add(1)
		go func(child core.Stage) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			default:
			}
			r := child.Run(rc.WithContext(ctx))
			select {
			case ch <- subResult{name: child.Name(), result: r}:
			case <-ctx.Done():
			}
		}(child)
	}

	go func() { wg.Wait(); close(ch) }()

	var lastErr error
	failed := 0
	for sr := range ch {
		if sr.result.IsSuccess() {
			cancel()
			return core.StageResult{
				Status:  core.StageSuccess,
				Next:    s.next,
				Outputs: sr.result.Outputs,
			}
		}
		lastErr = sr.result.Err
		failed++
		if failed == len(s.children) {
			return core.StageResult{
				Status: core.StageFailed,
				Err:    fmt.Errorf("all sub-stages failed, last error: %w", lastErr),
			}
		}
	}

	return core.StageResult{Status: core.StageFailed, Err: lastErr}
}

func (s *Stage) runBestEffort(rc *core.Context) core.StageResult {
	type subResult struct {
		name   string
		result core.StageResult
	}

	ch := make(chan subResult, len(s.children))
	var wg sync.WaitGroup

	for _, child := range s.children {
		wg.Add(1)
		go func(child core.Stage) {
			defer wg.Done()
			ch <- subResult{name: child.Name(), result: child.Run(rc)}
		}(child)
	}

	go func() { wg.Wait(); close(ch) }()

	merged := make(map[string]any)
	succeeded := 0
	var lastErr error

	for sr := range ch {
		if sr.result.IsFailed() {
			lastErr = sr.result.Err
			continue
		}
		if err := mergeOutputs(merged, sr.result.Outputs, s.opts.conflict); err != nil {
			return core.StageResult{Status: core.StageFailed, Err: err}
		}
		succeeded++
	}

	if succeeded == 0 {
		return core.StageResult{
			Status: core.StageFailed,
			Err:    fmt.Errorf("all sub-stages failed, last error: %w", lastErr),
		}
	}

	return core.StageResult{Status: core.StageSuccess, Next: s.next, Outputs: merged}
}
