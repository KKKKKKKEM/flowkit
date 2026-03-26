package core

import (
	"context"
	"time"
)

type SharedState map[string]any

// RunContext 实现 context.Context 接口，同时承载业务数据
// 这是整个框架的核心上下文对象，所有 Stage 通过它进行数据共享与信号传递
type RunContext struct {
	ctx       context.Context
	TraceID   string
	Values    SharedState // 执行过程中的中间产物与共享数据
	Tags      map[string]string
	StartedAt time.Time
}

func (rc *RunContext) Deadline() (deadline time.Time, ok bool) {
	return rc.ctx.Deadline()
}

func (rc *RunContext) Done() <-chan struct{} {
	return rc.ctx.Done()
}

func (rc *RunContext) Err() error {
	return rc.ctx.Err()
}

// Value 先查业务 Values，再落回到底层 context
func (rc *RunContext) Value(key interface{}) interface{} {
	if k, ok := key.(string); ok {
		if v, exist := rc.Values[k]; exist {
			return v
		}
	}
	return rc.ctx.Value(key)
}

// NewRunContext 创建一个新的 RunContext
func NewRunContext(ctx context.Context, traceID string) *RunContext {
	if ctx == nil {
		ctx = context.Background()
	}
	return &RunContext{
		ctx:       ctx,
		TraceID:   traceID,
		Values:    make(SharedState),
		Tags:      make(map[string]string),
		StartedAt: time.Now(),
	}
}

// WithTimeout 返回一个新的带超时的 RunContext
func (rc *RunContext) WithTimeout(d time.Duration) (*RunContext, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(rc.ctx, d)
	return &RunContext{
		ctx:       ctx,
		TraceID:   rc.TraceID,
		Values:    rc.Values,
		Tags:      rc.Tags,
		StartedAt: rc.StartedAt,
	}, cancel
}

// WithCancel 返回一个新的可取消的 RunContext
func (rc *RunContext) WithCancel() (*RunContext, context.CancelFunc) {
	ctx, cancel := context.WithCancel(rc.ctx)
	return &RunContext{
		ctx:       ctx,
		TraceID:   rc.TraceID,
		Values:    rc.Values,
		Tags:      rc.Tags,
		StartedAt: rc.StartedAt,
	}, cancel
}

// WithValue 返回一个新的包含值的 RunContext
func (rc *RunContext) WithValue(key string, val any) *RunContext {
	rc.Values[key] = val
	return rc
}

// Duration 返回从启动到现在的耗时
func (rc *RunContext) Duration() time.Duration {
	return time.Since(rc.StartedAt)
}
