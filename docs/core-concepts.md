# 核心概念

## App

`App[Req, Resp]` 是顶层执行单元，持有一个 `Invoke` 函数，其余全部通过 `core.Context` 传递。

```go
// core/app.go
type App[Req any, Resp any] interface {
    Invoke(rc *Context, req Req) (Resp, error)
}
```

`flowkit.NewApp` 接受一个函数直接构造 `App`：

```go
app := flowkit.NewApp(func(ctx *core.Context, req *MyReq) (*MyResp, error) {
    // 业务逻辑
    return &MyResp{}, nil
})
```

---

## Context

`core.Context` 实现 `context.Context` 接口，同时承载两层额外状态：

```
*Context
  ├── ctx context.Context   — 底层取消/超时信号
  ├── State *SharedState    — Stage 间共享的业务数据（线程安全）
  └── Runtime *Runtime
        ├── TraceID   string
        ├── StartedAt time.Time
        ├── Tags      map[string]string
        ├── TrackerProvider   — 进度追踪适配器
        └── InteractionPlugin — 交互适配器
```

### SharedState

所有 Stage 通过 `SharedState` 共享数据，推荐使用类型安全的辅助函数：

```go
// 写入
core.SetState(rc, "items", myItems)

// 读取（泛型，类型不匹配返回 zero + false）
items, ok := core.GetState[[]extract.Item](rc, "items")
```

也可以直接操作：

```go
rc.State.Set("key", value)
v, ok := rc.State.Get("key")
rc.State.Merge(map[string]any{"a": 1, "b": 2})
```

`Value(key)` 实现了 `context.Context` 接口：先查 `SharedState`，再落回底层 ctx。

### Context 派生

| 方法 | SharedState | TraceID | 适用场景 |
|------|-------------|---------|----------|
| `Fork(traceID)` | **新建**（独立） | 新 traceID | 独立子任务（如并发 fan-out 分支） |
| `Derive(traceID)` | **共享**父级 | 可选更换 | 同任务内区分 trace 范围 |
| `WithTimeout(d)` | 共享 | 不变 | 为某个 Stage 添加超时 |
| `WithCancel()` | 共享 | 不变 | 为某个 Stage 添加取消 |

```go
// fan.Stage 内部：每个并发分支用 Fork 隔离 SharedState
child := rc.Fork(uuid.NewString())
go runBranch(child)

// 同任务内子阶段：Derive 共享数据，换 traceID 区分日志
sub := rc.Derive("download-phase")
```

### TrackerProvider 与 Tracker

进度追踪通过 `Runtime.TrackerProvider` 注入，Stage 内调用：

```go
tracker := rc.Runtime.TrackerProvider.Track("download:file1", map[string]any{
    "total": int64(fileSize),
    "name":  "video.mp4",
})
tracker.Update(map[string]any{"current": bytesWritten})
tracker.Done()
```

`TrackerProvider` 接口：

```go
type TrackerProvider interface {
    Track(tag string, meta map[string]any) Tracker
    Wait()  // 等待所有 Tracker 完成
}

type Tracker interface {
    Update(d map[string]any)  // 更新进度
    Flush()                   // 强制刷新显示
    Done()                    // 标记完成
}
```

CLI 模式下由 `MPBTrackerProvider`（mpb 进度条）实现；HTTP/SSE 模式下由 `SSETrackerProvider` 实现，向客户端推送 `track` 事件。

### InteractionPlugin

人工介入点通过 `Runtime.InteractionPlugin` 注入：

```go
result, err := rc.Runtime.InteractionPlugin.Interact(rc, core.Interaction{
    Type:    core.InteractionTypeSelect,
    Payload: items,
    Message: "请选择要下载的内容",
})
```

接口：

```go
type InteractionPlugin interface {
    Interact(rc *Context, i Interaction) (*InteractionResult, error)
    FormatResult(rc *Context, i Interaction, result *InteractionResult) (*InteractionResult, error)
}
```

交互类型（`InteractionType`）：

| 值 | 含义 |
|----|------|
| `user_input` | 自由文本输入 |
| `approval` | 确认 / 拒绝 |
| `captcha` | 验证码 |
| `select` | 列表选择 |
| `custom` | 自定义类型 |

CLI 模式由 `CLIInteractionPlugin` 实现（读 stdin）；SSE 模式由 `SSEInteractionPlugin` 实现（推送事件，挂起 goroutine 直到客户端 POST `/answer`）。

---

## Stage

### 基础 Stage

```go
type Stage interface {
    Name() string
    Run(rc *Context) StageResult
}
```

`StageResult` 字段：

```go
type StageResult struct {
    Status  StageStatus        // success / skipped / retry / failed
    Next    string             // FSM 下一跳 Stage 名称（Linear 模式忽略）
    Outputs map[string]any     // 自动合并到 rc.State
    Metrics map[string]float64 // 指标数据（可选）
    Err     error
}
```

`Outputs` 在 Stage 返回后会被 Pipeline 自动写入 `rc.State`，下一个 Stage 可直接读取。

### TypedStage（类型安全适配器）

`TypedStage[In, Out]` 适合输入/输出类型明确的场景：

```go
type TypedStage[In any, Out any] interface {
    Name() string
    Exec(rc *Context, in In) (TypedResult[Out], error)
}

type TypedResult[Out any] struct {
    Output  Out
    Next    string
    Metrics map[string]float64
}
```

用 `NewTypedStage` 包装成标准 `Stage`，Pipeline 可以直接使用：

```go
stage := core.NewTypedStage(
    "transform",      // Stage 名称
    "selected",       // 从 SharedState 读取 In 的键
    "tasks",          // 将 Out 写入 SharedState 的键
    &myTypedStage{},  // 实现 TypedStage[Selected, Tasks]
)
```

执行时：
1. 从 `rc.State` 用 `keyIn` 读取 `In`，类型不匹配 → `StageFailed`
2. 调用 `Exec(rc, in)`
3. 将 `Out` 写入 `rc.State[keyOut]`，`Next` 和 `Metrics` 透传到 `StageResult`

---

## Middleware

Middleware 包裹 Stage 执行，签名：

```go
type StageRunner func(rc *Context, st Stage) StageResult
type Middleware  func(next StageRunner) StageRunner
```

实现示例（日志中间件）：

```go
func LogMiddleware(next core.StageRunner) core.StageRunner {
    return func(rc *core.Context, st core.Stage) core.StageResult {
        start := time.Now()
        result := next(rc, st)
        log.Printf("stage=%s status=%s elapsed=%s", st.Name(), result.Status, time.Since(start))
        return result
    }
}
```

多个 Middleware 通过 `core.Chain` 组合，**按注册顺序依次包裹**（外层先执行）：

```go
core.Chain(LogMiddleware, RetryMiddleware, MetricsMiddleware)
// 执行顺序：Log → Retry → Metrics → Stage → Metrics → Retry → Log
```

挂载到 Pipeline：

```go
p := pipeline.NewLinearPipeline("main")
p.Use(LogMiddleware, RetryMiddleware)
p.Add(stage1, stage2)
```

---

## StageStatus 语义

| 状态 | 含义 | Pipeline 行为 |
|------|------|---------------|
| `success` | 执行成功 | 继续下一个 Stage |
| `skipped` | 主动跳过 | 继续下一个 Stage（Linear）；跳到 Next（FSM） |
| `retry` | 请求重试 | 取决于 Middleware 是否处理；默认继续 |
| `failed` | 执行失败 | 停止 Pipeline，返回 `Err` |
