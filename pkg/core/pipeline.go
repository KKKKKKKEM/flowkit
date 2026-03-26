package core

import (
	"fmt"
	"time"
)

type PipelineMode string

const (
	ModeLinear   PipelineMode = "linear"
	ModeFSM      PipelineMode = "fsm"
	ModeParallel PipelineMode = "parallel"
	ModeDAG      PipelineMode = "dag"
)

// RunOptions 定义 Pipeline 执行参数
type RunOptions struct {
	FailFast    bool
	MaxParallel int
	TimeoutMs   int64
}

// RunReport 是 Pipeline 执行的完整报告
type RunReport struct {
	Mode         PipelineMode           `json:"mode,omitempty"`
	Success      bool                   `json:"success,omitempty"`
	TraceID      string                 `json:"trace_id,omitempty"`
	StageOrder   []string               `json:"stage_order,omitempty"`   // 执行顺序
	StageResults map[string]StageResult `json:"stage_results,omitempty"` // stage 名 -> result
	DurationMs   int64                  `json:"duration_ms,omitempty"`
}

// Pipeline 是框架的执行引擎
type Pipeline interface {
	// Mode 返回这个 pipeline 的执行模式
	Mode() PipelineMode
	// Register 注册一个或多个 stage
	Register(stages ...Stage) Pipeline
	// Run 从指定的入口 stage 开始执行，entry 是 stage 名
	// rc 既是 context 也是业务数据容器
	Run(rc *RunContext, entry string) (*RunReport, error)
}

// LinearPipeline 是顺序执行的线性管道
type LinearPipeline struct {
	stages map[string]Stage
	mw     []Middleware
}

func NewLinearPipeline() *LinearPipeline {
	return &LinearPipeline{
		stages: make(map[string]Stage),
	}
}

func (lp *LinearPipeline) Mode() PipelineMode {
	return ModeLinear
}

func (lp *LinearPipeline) Register(stages ...Stage) Pipeline {
	for _, s := range stages {
		lp.stages[s.Name()] = s
	}
	return lp
}

func (lp *LinearPipeline) Use(mw ...Middleware) *LinearPipeline {
	lp.mw = append(lp.mw, mw...)
	return lp
}

func (lp *LinearPipeline) Run(rc *RunContext, entry string) (*RunReport, error) {
	report := &RunReport{
		Mode:         ModeLinear,
		TraceID:      rc.TraceID,
		StageOrder:   []string{},
		StageResults: make(map[string]StageResult),
		DurationMs:   0,
	}

	start := time.Now()

	st, ok := lp.stages[entry]
	if !ok {
		return report, fmt.Errorf("entry stage not found: %s", entry)
	}

	runner := lp.makeStageRunner()
	for st != nil {
		// 检查是否已取消或超时
		if rc.Err() != nil {
			report.StageResults[st.Name()] = StageResult{
				Status: StageFailed,
				Err:    rc.Err(),
			}
			break
		}

		result := runner(rc, st)
		report.StageOrder = append(report.StageOrder, st.Name())
		report.StageResults[st.Name()] = result

		// 合并输出到共享状态
		for k, v := range result.Outputs {
			rc.Values[k] = v
		}

		if result.IsFailed() {
			report.Success = false
			break
		}

		st = nil // Linear 模式中，每个 stage 执行一次就结束
	}

	if report.Success {
		report.Success = true
	}

	report.DurationMs = time.Since(start).Milliseconds()
	return report, nil
}

func (lp *LinearPipeline) makeStageRunner() func(*RunContext, Stage) StageResult {
	runner := func(rc *RunContext, st Stage) StageResult {
		return st.Run(rc)
	}

	// 从后往前包裹中间件
	for i := len(lp.mw) - 1; i >= 0; i-- {
		runner = lp.mw[i](runner)
	}

	return runner
}

// FSMPipeline 是有向状态机模式的管道，通过 Stage.Result.Next 驱动
type FSMPipeline struct {
	stages    map[string]Stage
	mw        []Middleware
	maxVisits int
}

func NewFSMPipeline() *FSMPipeline {
	return &FSMPipeline{
		stages:    make(map[string]Stage),
		maxVisits: 100,
	}
}

func (fp *FSMPipeline) Mode() PipelineMode {
	return ModeFSM
}

func (fp *FSMPipeline) Register(stages ...Stage) Pipeline {
	for _, s := range stages {
		fp.stages[s.Name()] = s
	}
	return fp
}

func (fp *FSMPipeline) Use(mw ...Middleware) *FSMPipeline {
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

func (fp *FSMPipeline) Run(rc *RunContext, entry string) (*RunReport, error) {
	report := &RunReport{
		Mode:         ModeFSM,
		TraceID:      rc.TraceID,
		StageOrder:   []string{},
		StageResults: make(map[string]StageResult),
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
		for k, v := range result.Outputs {
			rc.Values[k] = v
		}

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

	if len(report.StageResults) > 0 && report.Success {
		report.Success = true
	}

	report.DurationMs = time.Since(start).Milliseconds()
	return report, nil
}

func (fp *FSMPipeline) makeStageRunner() func(*RunContext, Stage) StageResult {
	runner := func(rc *RunContext, st Stage) StageResult {
		return st.Run(rc)
	}

	// 从后往前包裹中间件
	for i := len(fp.mw) - 1; i >= 0; i-- {
		runner = fp.mw[i](runner)
	}

	return runner
}
