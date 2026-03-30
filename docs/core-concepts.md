# 核心概念

## App

`App` 是最顶层的可执行单元。

定义在：`core/app.go`

```go
type App[Req, Resp any] interface {
    Invoke(ctx *Context, req Req) (Resp, error)
}
```

顶层 `flowkit.App[Req, Resp]` 定义在 `app.go`，它是对核心 `Invoke` 的门面包装，提供：

- `App.CLI(...)`
- `App.Serve(...)`
- `App.Launch(...)`

核心业务逻辑只有一份，启动方式可以切换。

---

## Context

`core.Context` 是整个框架的运行时上下文。

它同时承担三种职责：

1. 标准 `context.Context` 能力
2. 共享状态容器
3. 运行时插件与追踪环境容器

### 组成

- `State`
  - `Set/Get/Merge`
- `Runtime`
  - `TraceID`
  - `StartedAt`
  - `Tags`
  - `InteractionPlugin`
  - `TrackerProvider`

### 常用方法

- `NewContext(ctx, traceID)`
- `WithTimeout(d)`
- `WithCancel()`
- `Fork(traceID)`
- `Derive(traceID)`

### 使用建议

- 用 `Fork()` 为子任务创建新的 trace scope
- 用共享状态在 stage 间传递中间结果
- 通过 Runtime 注入 tracker / interaction，而不是把 transport 细节塞进业务逻辑

---

## Stage

`Stage` 是最小执行单元。

```go
type Stage interface {
    Name() string
    Run(rc *Context) StageResult
}
```

返回 `StageResult`，包含：

- `Status`
- `Next`
- `Outputs`
- `Metrics`
- `Err`

其中：

- `Outputs` 会 merge 到共享状态
- `Next` 在 FSM 模式中驱动下一跳

---

## TypedStage

如果一个阶段有明确的输入输出边界，建议使用：

```go
type TypedStage[In any, Out any] interface {
    Name() string
    Exec(rc *Context, in In) (TypedResult[Out], error)
}
```

再通过 `core.NewTypedStage(...)` 适配成普通 `Stage`。

这也是当前仓库中 `extract` 和 `download` 的主要做法。

---

## Pipeline

`Pipeline` 是执行引擎。

```go
type Pipeline interface {
    Mode() Mode
    Register(stages ...Stage)
    Run(rc *Context, entry string) (*Report, error)
}
```

当前仓库提供两种实现：

- `LinearPipeline`
- `FSMPipeline`
