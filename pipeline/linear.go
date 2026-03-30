package pipeline

import (
	"fmt"
	"time"

	"github.com/KKKKKKKEM/flowkit/core"
)

// LinearPipeline 是顺序执行的线性管道
type LinearPipeline struct {
	order  []string
	stages map[string]core.Stage
	mw     []core.Middleware
}

func NewLinearPipeline() *LinearPipeline {
	return &LinearPipeline{
		order:  []string{},
		stages: make(map[string]core.Stage),
	}
}

func (lp *LinearPipeline) Mode() core.Mode {
	return core.ModeLinear
}

func (lp *LinearPipeline) Register(stages ...core.Stage) {
	for _, s := range stages {
		if _, exists := lp.stages[s.Name()]; !exists {
			lp.order = append(lp.order, s.Name())
		}
		lp.stages[s.Name()] = s
	}
}

func (lp *LinearPipeline) Use(mw ...core.Middleware) *LinearPipeline {
	lp.mw = append(lp.mw, mw...)
	return lp
}

func (lp *LinearPipeline) Run(rc *core.Context, entry string) (*core.Report, error) {
	report := &core.Report{
		Mode:         core.ModeLinear,
		TraceID:      rc.Runtime.TraceID,
		StageOrder:   []string{},
		StageResults: make(map[string]core.StageResult),
		DurationMs:   0,
	}

	start := time.Now()

	if _, ok := lp.stages[entry]; !ok {
		return report, fmt.Errorf("entry stage not found: %s", entry)
	}

	startIdx := -1
	for i, name := range lp.order {
		if name == entry {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		return report, fmt.Errorf("entry stage not found in registration order: %s", entry)
	}

	runner := lp.makeStageRunner()
	for _, stageName := range lp.order[startIdx:] {
		// 检查是否已取消或超时
		if rc.Err() != nil {
			report.StageResults[stageName] = core.StageResult{
				Status: core.StageFailed,
				Err:    rc.Err(),
			}
			report.StageOrder = append(report.StageOrder, stageName)
			report.Success = false
			break
		}

		st := lp.stages[stageName]
		result := runner(rc, st)
		report.StageOrder = append(report.StageOrder, stageName)
		report.StageResults[stageName] = result

		// 合并输出到共享状态
		rc.State.Merge(result.Outputs)

		if result.IsFailed() {
			report.Success = false
			break
		}
	}
	if len(report.StageOrder) > 0 {
		last := report.StageResults[report.StageOrder[len(report.StageOrder)-1]]
		report.Success = last.Status != core.StageFailed
	}

	report.DurationMs = time.Since(start).Milliseconds()
	return report, nil
}

func (lp *LinearPipeline) makeStageRunner() func(*core.Context, core.Stage) core.StageResult {
	runner := func(rc *core.Context, st core.Stage) core.StageResult {
		return st.Run(rc)
	}

	// 从后往前包裹中间件
	for i := len(lp.mw) - 1; i >= 0; i-- {
		runner = lp.mw[i](runner)
	}

	return runner
}
