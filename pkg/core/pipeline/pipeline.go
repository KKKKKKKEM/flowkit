package pipeline

import (
	"github.com/KKKKKKKEM/grasp/pkg/core"
)

type Mode string

const (
	ModeLinear   Mode = "linear"
	ModeFSM      Mode = "fsm"
	ModeParallel Mode = "parallel"
	ModeDAG      Mode = "dag"
)

// RunOptions 定义 Pipeline 执行参数
type RunOptions struct {
	FailFast    bool
	MaxParallel int
	TimeoutMs   int64
}

// RunReport 是 Pipeline 执行的完整报告
type RunReport struct {
	Mode         Mode                        `json:"mode,omitempty"`
	Success      bool                        `json:"success,omitempty"`
	TraceID      string                      `json:"trace_id,omitempty"`
	StageOrder   []string                    `json:"stage_order,omitempty"`   // 执行顺序
	StageResults map[string]core.StageResult `json:"stage_results,omitempty"` // stage 名 -> result
	DurationMs   int64                       `json:"duration_ms,omitempty"`
}

// Pipeline 是框架的执行引擎
type Pipeline interface {
	// Mode 返回这个 pipeline 的执行模式
	Mode() Mode
	// Register 注册一个或多个 stage
	Register(stages ...core.Stage) Pipeline
	// Run 从指定的入口 stage 开始执行，entry 是 stage 名
	// rc 既是 context 也是业务数据容器
	Run(rc *core.RunContext, entry string) (*RunReport, error)
}
