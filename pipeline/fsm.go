package pipeline

import (
	"fmt"
	"time"

	"github.com/KKKKKKKEM/flowkit/core"
)

// FSMPipeline 是有向状态机模式的管道，通过 Stage.Result.Next 驱动
type FSMPipeline struct {
	stages    map[string]core.Stage
	mw        []core.Middleware
	maxVisits int
}

func NewFSMPipeline() *FSMPipeline {
	return &FSMPipeline{
		stages:    make(map[string]core.Stage),
		maxVisits: 999,
	}
}

func (fp *FSMPipeline) Mode() core.Mode {
	return core.ModeFSM
}

func (fp *FSMPipeline) Register(stages ...core.Stage) {
	for _, s := range stages {
		fp.stages[s.Name()] = s
	}
}

func (fp *FSMPipeline) Use(mw ...core.Middleware) *FSMPipeline {
	fp.mw = append(fp.mw, mw...)
	return fp
}

// WithMaxVisits 设置单个 stage 最大访问次数，防止意外环路。
func (fp *FSMPipeline) WithMaxVisits(max int) *FSMPipeline {
	if max > 0 {
		fp.maxVisits = max
	}
	return fp
}

func (fp *FSMPipeline) Run(rc *core.Context, entry string) (*core.Report, error) {
	report := &core.Report{
		Mode:         core.ModeFSM,
		TraceID:      rc.Runtime.TraceID,
		StageOrder:   []string{},
		StageResults: make(map[string]core.StageResult),
		DurationMs:   0,
	}

	start := time.Now()

	st, ok := fp.stages[entry]
	if !ok {
		return report, fmt.Errorf("entry stage not found: %s", entry)
	}

	runner := fp.makeStageRunner()
	visited := make(map[string]int)

	for st != nil {
		// 检查是否已取消或超时
		if rc.Err() != nil {
			break
		}

		name := st.Name()

		// 环检测
		if visited[name] > fp.maxVisits {
			return report, fmt.Errorf("possible cycle detected: stage %s visited %d times (limit=%d)", name, visited[name], fp.maxVisits)
		}
		visited[name]++

		result := runner(rc, st)
		report.StageOrder = append(report.StageOrder, name)
		report.StageResults[name] = result

		// 合并输出到共享状态
		rc.State.Merge(result.Outputs)

		if result.IsFailed() {
			report.Success = false
			break
		}

		// 根据 Next 指定的下一个 stage
		nextName := result.Next
		if nextName == "" {
			// 没有指定下一步，说明流程结束
			report.Success = true
			break
		}

		st, ok = fp.stages[nextName]
		if !ok {
			return report, fmt.Errorf("next stage not found: %s", nextName)
		}
	}

	report.DurationMs = time.Since(start).Milliseconds()
	return report, nil
}

func (fp *FSMPipeline) makeStageRunner() func(*core.Context, core.Stage) core.StageResult {
	runner := func(rc *core.Context, st core.Stage) core.StageResult {
		return st.Run(rc)
	}

	// 从后往前包裹中间件
	for i := len(fp.mw) - 1; i >= 0; i-- {
		runner = fp.mw[i](runner)
	}

	return runner
}
