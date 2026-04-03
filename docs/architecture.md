# 架构概览

## 层次模型

```
┌─────────────────────────────────────────┐
│                  App                    │  ← 顶层门面，持有 Invoke 函数
│          App[Req, Resp]                 │
└──────────────┬──────────────────────────┘
               │ Invoke(ctx, req) → resp
               ▼
┌─────────────────────────────────────────┐
│               Pipeline                  │  ← 编排 Stage 的执行顺序
│  LinearPipeline │ FSMPipeline           │
└──────────────┬──────────────────────────┘
               │ Run(ctx) StageResult
               ▼
┌─────────────────────────────────────────┐
│                 Stage                   │  ← 原子业务单元
│  Stage │ TypedStage[In, Out]            │
└──────────────┬──────────────────────────┘
               │ 读写 SharedState
               ▼
┌─────────────────────────────────────────┐
│              core.Context               │  ← 跨 Stage 共享状态 + 运行时
│  context.Context + SharedState          │
│  + TrackerProvider + InteractionPlugin  │
└─────────────────────────────────────────┘
```

## 传输层解耦

App 本身不感知传输协议。相同的 `App.Invoke` 可被两类 transport 调用：

```
           ┌─ CLI ─────────────────────────────────────────┐
           │  os.Args → CLIBuilder → Req                   │
           │  Resp → 打印到 stdout                          │
  App ─────┤                                               │
           └─ HTTP / SSE ───────────────────────────────────┘
              POST /invoke → JSON Body → Req
              SSE /stream  → session → 流式事件推送
              POST /answer → 交互回传
```

CLI 使用 `CLITrackerProvider`（终端 mpb 进度条）和 `CLIInteractionPlugin`（stdin 提示输入）。

SSE 使用 `SSETrackerProvider`（向客户端推送 `track` 事件）和 `SSEInteractionPlugin`（向客户端推送 `interact` 事件，挂起直到 `/answer` 回传）。

## 模块职责

| 包 | 职责 |
|----|------|
| `app.go` | 顶层门面：`NewApp`、`Launch`、`CLI`、`Serve` |
| `core/` | 纯接口定义：`App`、`Context`、`Pipeline`、`Stage`、`Middleware`、`InteractionPlugin`、`TrackerProvider` |
| `pipeline/` | 执行器实现：`LinearPipeline`、`FSMPipeline` |
| `server/` | HTTP transport：简单 req/resp（`http.go`）和 SSE 会话（`sse.go` + `sse/`） |
| `stages/cond` | 条件跳转 Stage，配合 FSM 使用 |
| `stages/fan` | 并发 fan-out Stage |
| `stages/extract` | 内容提取 Stage，URL 匹配 → Parser → `[]Item` |
| `stages/download` | 批量下载 Stage，分片 + 并发 + 失败策略 |
| `x/grasp` | 完整应用示例，组合 extract + download |

## 数据流（以 x/grasp 为例）

```
HTTP POST /invoke
  │
  ├─ 创建 core.Context（含 SSETrackerProvider + SSEInteractionPlugin）
  │
  ▼
App.Invoke(ctx, req)
  │
  ▼
GraspPipeline.Run(ctx)
  │
  ├─ Stage 1: extract.Stage
  │    读 req.URL → 匹配 Parser → 写 ctx.SharedState["items"]
  │    同时 → ctx.Track("extract", ...)  → SSE track 事件
  │
  ├─ Stage 2: select.Stage（交互）
  │    读 ctx.SharedState["items"]
  │    → ctx.Interact(...)              → SSE interact 事件（挂起）
  │    ← POST /answer                  → 恢复执行
  │    写 ctx.SharedState["selected"]
  │
  ├─ Stage 3: transform.Stage
  │    读 ctx.SharedState["selected"] → 写 ctx.SharedState["tasks"]
  │
  └─ Stage 4: download.Stage
       读 ctx.SharedState["tasks"]
       → ctx.Track("dl:*", ...)         → SSE track 事件（进度）
       → SSE done 事件
```

## Context 生命周期

```
请求到达
  │
  ├─ server 层创建根 Context（NewContext）
  │
  ├─ Pipeline 直接传递同一 Context 给各 Stage
  │
  └─ Stage 可调用：
       ctx.Fork(traceID)    ← 新 SharedState（独立子任务）
       ctx.Derive(traceID)  ← 共享 SharedState（同一任务，新 trace）
```

`Fork` 适用于完全独立的子任务（如 fan.Stage 并发分支）；`Derive` 适用于需要共享上下文但要区分 trace 的场景。
