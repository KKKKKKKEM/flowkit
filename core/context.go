package core

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type SharedState struct {
	mu   sync.RWMutex
	data map[string]any
}

func NewSharedState() *SharedState {
	return &SharedState{data: make(map[string]any)}
}

func (s *SharedState) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = val
}

func (s *SharedState) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *SharedState) Merge(values map[string]any) {
	if len(values) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range values {
		s.data[k] = v
	}
}

type Runtime struct {
	TraceID           string
	StartedAt         time.Time
	Tags              map[string]string
	InteractionPlugin InteractionPlugin
	TrackerProvider   TrackerProvider
}

func newRuntime(traceID string) *Runtime {
	return &Runtime{
		TraceID:   traceID,
		StartedAt: time.Now(),
		Tags:      make(map[string]string),
	}
}

func (r *Runtime) Clone() *Runtime {
	if r == nil {
		return newRuntime("")
	}
	tags := make(map[string]string, len(r.Tags))
	for k, v := range r.Tags {
		tags[k] = v
	}
	return &Runtime{
		TraceID:           r.TraceID,
		StartedAt:         r.StartedAt,
		Tags:              tags,
		InteractionPlugin: r.InteractionPlugin,
		TrackerProvider:   r.TrackerProvider,
	}
}

// Context 实现 context.Context 接口，同时承载业务数据
// 这是整个框架的核心上下文对象，所有 Stage 通过它进行数据共享与信号传递
type Context struct {
	ctx     context.Context
	State   *SharedState
	Runtime *Runtime
}

func (rc *Context) Deadline() (deadline time.Time, ok bool) {
	return rc.ctx.Deadline()
}

func (rc *Context) Done() <-chan struct{} {
	return rc.ctx.Done()
}

func (rc *Context) Err() error {
	return rc.ctx.Err()
}

// Value 先查业务 Values，再落回到底层 context
func (rc *Context) Value(key interface{}) interface{} {
	if k, ok := key.(string); ok {
		if v, exist := rc.State.Get(k); exist {
			return v
		}
	}
	return rc.ctx.Value(key)
}

// NewContext 创建一个新的 Context
func NewContext(ctx context.Context, traceID string) *Context {
	if ctx == nil {
		ctx = context.Background()
	}

	if traceID == "" {
		traceID = uuid.NewString()
	}

	return &Context{
		ctx:     ctx,
		State:   NewSharedState(),
		Runtime: newRuntime(traceID),
	}
}

func (rc *Context) WithContext(ctx context.Context) *Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Context{
		ctx:     ctx,
		State:   rc.State,
		Runtime: rc.Runtime,
	}
}

// WithTimeout 返回一个新的带超时的 Context
func (rc *Context) WithTimeout(d time.Duration) (*Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(rc.ctx, d)
	return rc.WithContext(ctx), cancel
}

// WithCancel 返回一个新的可取消的 Context
func (rc *Context) WithCancel() (*Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(rc.ctx)
	return rc.WithContext(ctx), cancel
}

func (rc *Context) Fork(traceID string) *Context {
	if traceID == "" {
		traceID = rc.Runtime.TraceID
	}
	runtime := rc.Runtime.Clone()
	runtime.TraceID = traceID
	return &Context{
		ctx:     rc,
		State:   NewSharedState(),
		Runtime: runtime,
	}
}

func (rc *Context) Derive(traceID string) *Context {
	child := rc.WithContext(rc)
	if traceID != "" {
		child.Runtime = rc.Runtime.Clone()
		child.Runtime.TraceID = traceID
	}
	return child
}

// Duration 返回从启动到现在的耗时
func (rc *Context) Duration() time.Duration {
	return time.Since(rc.Runtime.StartedAt)
}
