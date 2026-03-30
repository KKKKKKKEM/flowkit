# Flowkit

Flowkit 是一个面向 Go 的轻量工作流/流水线框架，用来把一段核心业务逻辑包装成：

- 可组合的 `Stage`
- 可编排的 `Pipeline`
- 可直接启动的 `App`
- 同时支持 **CLI** 与 **HTTP/SSE** 两种运行入口

它的核心目标不是做一个“大而全”的 DAG 平台，而是提供一套足够清晰、类型友好、可嵌入应用的执行模型，让你可以把：

- 解析
- 分支
- 并发
- 下载
- 交互
- 进度追踪

这些能力组织成一套统一的执行流。

---

## 目录

- [文档目录](#文档目录)
- [核心特性](#核心特性)
- [项目结构](#项目结构)
- [核心概念](#核心概念)
  - [App](#app)
  - [Context](#context)
  - [Stage 与 TypedStage](#stage-与-typedstage)
  - [Pipeline](#pipeline)
- [启动方式](#启动方式)
  - [Launch](#launch)
  - [CLI](#cli)
  - [Server](#server)
- [内置 Pipeline 模式](#内置-pipeline-模式)
- [内置 Stages](#内置-stages)
- [快速开始](#快速开始)
  - [1. 创建一个 App](#1-创建一个-app)
  - [2. 启动为 CLI 或 Server](#2-启动为-cli-或-server)
  - [3. 运行语义](#3-运行语义)
- [真实示例：x/grasp](#真实示例xgrasp)
- [HTTP / SSE 交互模型](#http--sse-交互模型)
- [推荐的扩展方式](#推荐的扩展方式)
- [当前仓库现状与注意事项](#当前仓库现状与注意事项)

---

## 文档目录

更细分的文档已拆分到 `docs/`：

- [`docs/README.md`](./docs/README.md)
- [`docs/architecture.md`](./docs/architecture.md)
- [`docs/core-concepts.md`](./docs/core-concepts.md)
- [`docs/launch-and-runtime.md`](./docs/launch-and-runtime.md)
- [`docs/pipelines.md`](./docs/pipelines.md)
- [`docs/stages.md`](./docs/stages.md)
- [`docs/server-and-sse.md`](./docs/server-and-sse.md)
- [`docs/grasp.md`](./docs/grasp.md)
- [`docs/extending-flowkit.md`](./docs/extending-flowkit.md)
- [`docs/notes.md`](./docs/notes.md)

---

## 核心特性

- **传输层无关**：同一个 `App.Invoke` 可以跑在 CLI，也可以跑在 HTTP/SSE 上。
- **类型友好**：支持 `TypedStage` 和 typed adapter，把阶段输入输出从共享状态里安全取出。
- **上下文统一**：`core.Context` 同时承载取消信号、共享状态、Trace 信息、交互插件与追踪器。
- **可组合执行模型**：支持线性流水线与 FSM 状态跳转。
- **内置常见能力**：条件分支、并发 fan-out、提取、下载。
- **可交互运行**：同一套业务逻辑既可以在终端交互，也可以通过 SSE 会话在 Web 端交互。

---

## 项目结构

```text
.
├── app.go                # 顶层 App 门面：Launch / CLI / Serve
├── cli/                  # CLI transport adapter
├── core/                 # 核心抽象：App / Context / Pipeline / Stage / Interaction / Tracker
├── pipeline/             # Pipeline 执行器实现（Linear / FSM）
├── server/               # HTTP / SSE transport adapter
├── stages/               # 内置 stage 实现
│   ├── cond/             # 条件跳转
│   ├── download/         # 下载阶段
│   ├── extract/          # 提取阶段
│   ├── fan/              # 并发聚合阶段
│   └── internal/defaults/# 默认值合并辅助
├── x/grasp/              # 基于 Flowkit 构建的真实应用示例
└── examples/grasp/       # 最小启动示例
```

### 你最先应该看的文件

- `app.go`
- `core/context.go`
- `core/stage.go`
- `core/pipeline.go`
- `pipeline/linear.go`
- `pipeline/fsm.go`
- `x/grasp/pipeline.go`

---

## 核心概念

### App

`App` 是最顶层的可执行单元。

定义在：`core/app.go`

```go
type App[Req, Resp any] interface {
    Invoke(ctx *Context, req Req) (Resp, error)
}
```

顶层 `flowkit.App[Req, Resp]` 定义在 `app.go`，它是对核心 `Invoke` 的门面包装，提供三个入口：

- `App.CLI(...)`
- `App.Serve(...)`
- `App.Launch(...)`

也就是说，**业务逻辑只有一份**，启动方式可以切换。

---

### Context

`core.Context` 是整个框架最重要的运行时对象，定义在：`core/context.go`。

它同时承担三种职责：

1. **标准 context 能力**
   - Deadline
   - Done
   - Err
   - cancellation / timeout

2. **共享状态容器**
   - `State.Set/Get/Merge`
   - 不同 stage 之间通过共享状态传递数据

3. **运行时环境**
   - `TraceID`
   - `TrackerProvider`
   - `InteractionPlugin`
   - Tags / StartedAt

### 关键方法

- `NewContext(ctx, traceID)`：创建根上下文
- `WithTimeout(d)` / `WithCancel()`：派生带控制能力的上下文
- `Fork(traceID)`：创建新的 trace 子上下文，常用于子阶段/子任务
- `Derive(traceID)`：派生并保留共享运行时上下文

---

### Stage 与 TypedStage

定义在：`core/stage.go`

#### 普通 Stage

```go
type Stage interface {
    Name() string
    Run(rc *Context) StageResult
}
```

这是最小执行单元，直接读写 `Context` 中的共享状态。

#### TypedStage

```go
type TypedStage[In any, Out any] interface {
    Name() string
    Exec(rc *Context, in In) (TypedResult[Out], error)
}
```

适合更明确的输入输出边界。

Flowkit 提供 `core.NewTypedStage(...)`，把 `TypedStage` 适配成普通 `Stage`，定义在：`core/adapter.go`。

这也是当前仓库里数据处理类 stage 的主要做法，比如：

- `stages/extract`
- `stages/download`

### StageResult

单个阶段返回：

- `Status`
- `Next`
- `Outputs`
- `Metrics`
- `Err`

其中：

- `Outputs` 会 merge 到共享状态
- `Next` 在 FSM 模式中驱动跳转

---

### Pipeline

定义在：`core/pipeline.go`

```go
type Pipeline interface {
    Mode() Mode
    Register(stages ...Stage)
    Run(rc *Context, entry string) (*Report, error)
}
```

Flowkit 当前仓库提供两种执行器：

- `pipeline.NewLinearPipeline()`
- `pipeline.NewFSMPipeline()`

执行结果统一返回 `core.Report`，包含：

- 执行模式
- trace id
- stage 执行顺序
- 每个 stage 的结果
- 总耗时

---

## 启动方式

### Launch

`App.Launch(...)` 是推荐的统一入口。

当前默认支持两种模式：

- `run ...` → CLI 模式
- `serve --addr=:8080` → HTTP 模式

并保留一个便捷行为：

- 裸参数默认等价于 `run ...`

例如：

```bash
app run -url https://example.com
app serve --addr=:8080
app -url https://example.com
```

`Launch` 的配置模型是显式双配置：

- 一份 CLI config
- 一份 Serve config

Launch 自己只负责：

- 模式解析
- 启动分发

不会再把 server 逻辑藏进 CLI adapter 里。

---

### CLI

`App.CLI(...)` 是纯 CLI transport adapter。

定义行为在：

- `app.go`
- `cli/api.go`

它负责：

- 从 args 构建请求对象
- 创建 `core.Context`
- 注入 tracker / interaction plugin
- 调用 `App.Invoke`
- 将结果输出到 stdout

CLI 相关 option 包括：

- `WithCLIBuilder(...)`
- `WithCLIArgs(...)`
- `WithTrackerProvider(...)`
- `WithInteractionPlugin(...)`
- `WithOnResult(...)`
- `WithOnError(...)`

---

### Server

`App.Serve(...)` 是服务端入口。

当前 `App.Serve(...)` 默认是基于 `server.SSE(...)` 的，也就是说：

- 它不是单纯的 REST 包装
- 而是一个支持会话、流式事件、交互回传的 HTTP/SSE 运行时

定义在：

- `server/sse.go`

除此之外，仓库里还提供了一个独立的普通 HTTP adapter：

- `server/http.go`

它可以在你不需要 SSE 会话与交互回传时单独使用。

### `server/http.go`

提供普通 HTTP request/response 适配：

- 请求绑定到 `Req`
- 创建 `core.Context`
- 调用 `App.Invoke`
- JSON 返回 `Resp`

### `server/sse.go`

提供会话式 SSE adapter：

- `POST {path}/stream`
  - 创建或恢复会话
  - 启动 App 执行
  - 通过 SSE 推送 tracker / interaction / done / error

- `POST {path}/answer`
  - 将前端交互结果回注到运行中的 session

这也是 Flowkit 支持“同一套业务逻辑，终端交互与 Web 交互共存”的关键。

---

## 内置 Pipeline 模式

### 1. Linear Pipeline

定义在：`pipeline/linear.go`

特点：

- 按注册顺序执行
- 从 `entry` 开始往后执行
- 任一 stage 失败则中断
- 每个 stage 的 `Outputs` 自动 merge 到共享状态

适合：

- 顺序处理流程
- 提取 → 选择 → 转换 → 下载 这类直线型业务

---

### 2. FSM Pipeline

定义在：`pipeline/fsm.go`

特点：

- 根据 `StageResult.Next` 决定下一跳
- 支持状态机式编排
- 内置 `maxVisits` 防环保护

适合：

- 条件分支
- 多状态流转
- 可恢复/可重复进入的流程

---

## 内置 Stages

### `stages/cond`

条件跳转 stage，适用于 FSM。

- 顺序评估分支
- 命中第一个条件即跳转
- 未命中走 fallback

定义在：`stages/cond/stage.go`

---

### `stages/fan`

并发子阶段执行器。

支持：

- `wait all + fail fast`
- `wait any`
- `best effort`
- 输出冲突合并策略

定义在：`stages/fan/stage.go`

适合：

- 扇出执行多个子 stage
- 并行探测/并行抓取/并行提取

---

### `stages/extract`

typed data stage。

职责：

- 根据 URL / forced parser 选择 parser
- 执行内容提取
- 输出 `[]Item`

当前支持：

- 默认值解析 (`ResolveOpts`)
- parser 挂载 (`Mount`)
- typed stage adapter

---

### `stages/download`

typed data stage。

职责：

- 接收下载任务列表
- 根据 URI 派发给 downloader
- 执行批量下载

当前已经具备的语义：

- batch 级失败策略
  - `BatchFailFast`
  - `BatchBestEffort`
- batch 级最大并发
- segment 级并发下载
- 结构化 `BatchError`
- 按代理配置复用 HTTP transport
- 统一 defaults / resolved opts 机制

---

## 快速开始

### 1. 创建一个 App

最小示例：

```go
package main

import (
    "log"
    "github.com/KKKKKKKEM/flowkit"
    "github.com/KKKKKKKEM/flowkit/core"
)

type Req struct {
    Name string `json:"name"`
}

type Resp struct {
    Message string `json:"message"`
}

func main() {
    app := flowkit.NewApp(func(ctx *core.Context, req *Req) (*Resp, error) {
        return &Resp{Message: "hello " + req.Name}, nil
    })

    if err := app.Launch(
        flowkit.WithLaunchCLIOptions(
            flowkit.WithCLIBuilder(func(args []string) (*Req, error) {
                return &Req{Name: "world"}, nil
            }),
        ),
    ); err != nil {
        log.Fatal(err)
    }
}
```

---

### 2. 启动为 CLI 或 Server

#### CLI

```go
app.CLI(
    flowkit.WithCLIBuilder(buildCLI),
)
```

#### Server

```go
app.Serve(":8080",
    flowkit.WithPath("/app"),
)
```

#### Launch

```go
app.Launch(
    flowkit.WithDefaultHTTPAddr(":8080"),
    flowkit.WithLaunchCLIOptions(
        flowkit.WithCLIBuilder(buildCLI),
    ),
    flowkit.WithLaunchServeOptions(
        flowkit.WithPath("/app"),
    ),
)
```

---

### 3. 运行语义

### CLI 模式

```bash
app run ...
app ...
```

### Server 模式

```bash
app serve --addr=:8080
```

---

## 真实示例：x/grasp

`x/grasp` 是当前仓库里最完整的实际应用。

入口示例：`examples/grasp/main.go`

```go
func main() {
    p := grasp.NewGraspPipeline()

    if err := p.Launch(); err != nil {
        log.Fatal(err)
    }
}
```

### 它做了什么

`x/grasp` 组合了：

- `extract.Stage`
- `download.Stage`
- CLI interaction plugin
- MPB tracker provider

并提供一个完整的资源抓取/下载流程。

### `grasp.Task`

定义在：`x/grasp/task.go`

主要配置包括：

- `URLs`
- `Proxy`
- `Timeout`
- `Retry`
- `Headers`
- `Extract.MaxRounds`
- `Extract.WorkerConcurrency`
- `Download.TaskConcurrency`
- `Download.SegmentConcurrency`
- `Download.BestEffort`
- `Download.ChunkSize`

### `grasp` CLI 参数

定义在：`x/grasp/cli.go`

当前支持：

- `-url`
- `-proxy`
- `-timeout`
- `-retry`
- `-header`
- `-rounds`
- `-extract-concurrency`
- `-dest`
- `-overwrite`
- `-download-task-concurrency`
- `-best-effort`
- `-download-segment-concurrency`
- `-chunk-size`

这也是当前仓库里最推荐参考的接入样例。

---

## HTTP / SSE 交互模型

Flowkit 的 server 不是简单把结果 JSON 返回就结束。

当前 server 侧更完整的能力来自 SSE：

### 1. 启动会话

`POST {path}/stream`

作用：

- 创建 SSE session
- 启动应用执行
- 推送实时事件

### 2. 交互回答

`POST {path}/answer`

作用：

- 将用户对 `InteractionPlugin` 的回答发送回运行中的 session

请求体格式：

```json
{
  "interaction_id": "...",
  "result": { ... }
}
```

### 3. Session 标识

通过 header：

- `SESSION-ID`
- `LAST-EVENT-ID`

来恢复会话和断线续流。

### 为什么这很重要

因为这意味着：

- 终端里的“等待用户选择”
- 在 Web 模式下可以变成“挂起流程并等待浏览器答复”

这也是 Flowkit 在交互式工作流里最有价值的一点。

---

## 推荐的扩展方式

### 1. 新建一个 App

如果你只是想把一段业务逻辑同时暴露为 CLI 和 HTTP：

- 用 `flowkit.NewApp(...)`
- 用 `Launch()` 统一启动

### 2. 新建一个 TypedStage

如果你有清晰的输入输出：

- 实现 `TypedStage[In, Out]`
- 用 `core.NewTypedStage(...)` 适配为普通 stage

### 3. 新建一个 Pipeline

如果你需要组织多个阶段：

- 简单顺序流 → `pipeline.NewLinearPipeline()`
- 状态驱动流 → `pipeline.NewFSMPipeline()`

### 4. 新建 transport-specific 插件

如果你想自定义：

- 进度推送方式
- 用户交互方式

可以实现：

- `core.TrackerProvider`
- `core.InteractionPlugin`

CLI 和 SSE 就是当前两套现成实现。

---

## 当前仓库现状与注意事项

### 1. 顶层启动模型已经比较完整

当前推荐直接使用：

- `App.Launch()`

而不是自己在 CLI 里做模式切换。

### 2. `x/grasp` 是当前最重要的参考实现

如果你想看：

- 如何组装 pipeline
- 如何配置 Launch
- 如何接入 tracker / interaction
- 如何做真实下载流程

优先读：`x/grasp/`

### 3. 当前仓库存在 Go toolchain 配置问题

`go.mod` 当前写的是：

```go
go 1.25.0
```

如果你的本地工具链版本较低，可能会导致：

- `go list`
- `gopls`
- 部分构建/诊断

直接失败。

在当前开发环境里，这一点已经会影响完整包加载验证。

### 4. 文档与代码同步建议

如果后续继续演进，优先保持以下三处文档同步：

- `app.go` 中的 Launch 语义
- `x/grasp/cli.go` 中的 CLI 参数
- `server/sse.go` 中的会话接口约定

---

## 最后

如果你是第一次接触这个仓库，推荐阅读顺序是：

1. `app.go`
2. `core/context.go`
3. `core/stage.go`
4. `pipeline/linear.go`
5. `pipeline/fsm.go`
6. `x/grasp/pipeline.go`
7. `x/grasp/task.go`
8. `server/sse.go`

这样可以最快理解：

- Flowkit 的执行模型
- Launch / CLI / Serve 的关系
- Stage / Pipeline 的职责边界
- 一个真实应用如何构建出来
