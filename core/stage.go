package core

import (
	"context"
	"errors"
)

type StageStatus string

const (
	StageSuccess StageStatus = "success"
	StageSkipped StageStatus = "skipped"
	StageRetry   StageStatus = "retry"
	StageFailed  StageStatus = "failed"
)

// Stage 是工作流中的最小执行单元
// 它接收 Context（实现了 context.Context），返回 StageResult
type Stage interface {
	// Name 返回这个 stage 的唯一标识，用于日志、依赖图、下一跳指引
	Name() string
	// Run 执行业务逻辑，rc 同时承载信号（Done、Err、Deadline）和业务数据（Values、Tags）
	Run(rc *Context) StageResult
}

type TypedStage[In any, Out any] interface {
	Name() string
	Exec(rc *Context, in In) (result TypedResult[Out], err error)
}

// StageResult 是单个 Stage 执行的结果
type StageResult struct {
	// Status 本次执行的结果状态
	Status StageStatus `json:"status,omitempty"`
	// Next 用于 FSM/DAG 模式指定下一个 stage 的名字，Linear 模式可忽略
	Next string `json:"next,omitempty"`
	// Outputs 业务层输出的数据，会合并到 rc.Values 供后续 stage 读取
	Outputs map[string]any `json:"outputs,omitempty"`
	// Metrics 本次执行收集的指标（例如下载字节数、耗时、重试次数）
	Metrics map[string]float64 `json:"metrics,omitempty"`
	// Err 如果执行失败则设置错误
	Err error `json:"err,omitempty"`
}

type TypedResult[Out any] struct {
	Output  Out
	Next    string
	Metrics map[string]float64
}

// IsSuccess 便利方法：检查是否成功
func (sr *StageResult) IsSuccess() bool {
	return sr.Status == StageSuccess
}

// IsFailed 便利方法：检查是否失败
func (sr *StageResult) IsFailed() bool {
	return sr.Status == StageFailed
}

// IsTerminal 便利方法：检查是否终止状态（成功或失败）
func (sr *StageResult) IsTerminal() bool {
	return sr.Status == StageSuccess || sr.Status == StageFailed
}

// ErrorClass 用于错误分类和处理策略
type ErrorClass string

const (
	// ErrTransient 短暂错误，可重试（网络超时、临时服务不可用）
	ErrTransient ErrorClass = "transient"
	// ErrBusiness 业务错误，不应重试（参数错误、业务规则违反）
	ErrBusiness ErrorClass = "business"
	// ErrFatal 致命错误，立即终止（panic、资源耗尽）
	ErrFatal ErrorClass = "fatal"
)

// ErrorPolicy 定义错误分类和重试策略
type ErrorPolicy interface {
	// Classify 将错误分类为不同等级
	Classify(err error) ErrorClass
	// ShouldRetry 判断是否应该重试
	ShouldRetry(stage string, err error, attempt int) bool
}

// DefaultErrorPolicy 提供默认的错误分类
type DefaultErrorPolicy struct {
	MaxRetries int
}

func (p *DefaultErrorPolicy) Classify(err error) ErrorClass {
	if err == nil {
		return ErrTransient
	}
	// 简单的启发式分类：context 错误通常是致命的
	if errors.Is(err, context.Canceled) {
		return ErrFatal
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTransient // 允许重试
	}
	return ErrBusiness // 默认业务错误不重试
}

func (p *DefaultErrorPolicy) ShouldRetry(stage string, err error, attempt int) bool {
	if attempt >= p.MaxRetries {
		return false
	}
	class := p.Classify(err)
	return class == ErrTransient
}
