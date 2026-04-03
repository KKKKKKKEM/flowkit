# Pipeline 模式

Flowkit 提供两种 Pipeline 实现，均定义在 `pipeline/` 包，支持相同的 Middleware 机制。

## LinearPipeline

按注册顺序依次执行 Stage，任意 Stage 失败即停止。

```go
p := pipeline.NewLinearPipeline()
p.Use(LogMiddleware, MetricsMiddleware)  // 可选：挂载 Middleware
p.Register(stage1, stage2, stage3)

report, err := p.Run(rc, "stage1")  // entry 指定起始 Stage 名称
```

**执行规则：**
- 从 `entry` Stage 开始，按注册顺序向后执行（`entry` 前的 Stage 被跳过）
- 每个 Stage 的 `Outputs` 自动合并到 `rc.State`，供后续 Stage 读取
- `StageResult.Next` 字段**被忽略**（线性模式不做跳转）
- `rc.Err() != nil`（ctx 取消/超时）→ 当前 Stage 标记 `failed` 并停止
- `status == failed` → 停止整个 Pipeline

**适用场景：**步骤固定、顺序明确的流水线（extract → transform → download）。

---

## FSMPipeline

由 `StageResult.Next` 驱动的状态机，Stage 可以任意跳转。

```go
p := pipeline.NewFSMPipeline()
p.WithMaxVisits(10)             // 可选：防环保护（默认 999）
p.Use(LogMiddleware)
p.Register(stageA, stageB, stageC)

report, err := p.Run(rc, "stageA")
```

**执行规则：**
- 从 `entry` Stage 开始
- 执行完当前 Stage 后，读取 `result.Next`：
  - `Next == ""` → Pipeline 正常结束（`success`）
  - `Next == "stageName"` → 跳转到该 Stage
  - `Next` 指向未注册的 Stage → 返回 error
- 单个 Stage 访问次数超过 `maxVisits`（默认 999）→ 返回 error（防止意外环路）
- `status == failed` → 立即停止

**典型用法（条件分支）：**

```go
// cond.Stage 根据条件返回不同的 Next
type RouterStage struct{}

func (r *RouterStage) Name() string { return "router" }
func (r *RouterStage) Run(rc *core.Context) core.StageResult {
    items, _ := core.GetState[[]Item](rc, "items")
    if len(items) == 0 {
        return core.StageResult{Status: core.StageSuccess, Next: "no-result"}
    }
    return core.StageResult{Status: core.StageSuccess, Next: "select"}
}

p := pipeline.NewFSMPipeline()
p.Register(&RouterStage{}, selectStage, noResultStage, downloadStage)
```

**适用场景：**需要条件跳转、重试循环、多分支的复杂流程。

---

## Middleware

两种 Pipeline 的 Middleware 机制完全相同。

### 注册

```go
p.Use(mw1, mw2, mw3)
// 执行顺序：mw1 → mw2 → mw3 → Stage → mw3 → mw2 → mw1
```

`Use` 返回 Pipeline 自身，支持链式调用：

```go
p := pipeline.NewLinearPipeline().Use(LogMiddleware).Use(RetryMiddleware)
```

### 签名

```go
type StageRunner func(rc *core.Context, st core.Stage) core.StageResult
type Middleware  func(next StageRunner) StageRunner
```

### 示例：日志 + 重试 Middleware

```go
// 日志
func LogMiddleware(next core.StageRunner) core.StageRunner {
    return func(rc *core.Context, st core.Stage) core.StageResult {
        start := time.Now()
        result := next(rc, st)
        log.Printf("[%s] stage=%s status=%s elapsed=%dms",
            rc.Runtime.TraceID, st.Name(), result.Status, time.Since(start).Milliseconds())
        return result
    }
}

// 简单重试（transient error 最多重试 3 次）
func RetryMiddleware(next core.StageRunner) core.StageRunner {
    return func(rc *core.Context, st core.Stage) core.StageResult {
        for i := 0; i < 3; i++ {
            result := next(rc, st)
            if !result.IsFailed() || !isTransient(result.Err) {
                return result
            }
            time.Sleep(time.Duration(i+1) * time.Second)
        }
        return next(rc, st)
    }
}
```

### core.Chain（组合多个 Middleware）

```go
combined := core.Chain(LogMiddleware, RetryMiddleware, MetricsMiddleware)
p.Use(combined)
```

`Chain` 返回单个 `Middleware`，效果等同于依次 `Use`。

---

## Report

`Pipeline.Run` 返回 `*core.Report`，包含本次执行的完整记录：

```go
type Report struct {
    Mode         core.Mode                    // "linear" / "fsm"
    TraceID      string
    StageOrder   []string                     // 实际执行顺序
    StageResults map[string]core.StageResult  // 每个 Stage 的结果
    DurationMs   int64
    Success      bool
}
```

`StageOrder` 仅包含**实际执行过**的 Stage（跳过的 entry 前 Stage 不计入）。

---

## 选择建议

| 需求 | 推荐 |
|------|------|
| 步骤固定、顺序执行 | `LinearPipeline` |
| 需要条件跳转 / 循环重试 | `FSMPipeline` |
| 两者混用 | 内层用 Linear，外层 App 按情况选 FSM |
