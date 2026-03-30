# 架构概览

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

## 模块职责

### `app.go`

顶层门面，统一封装：

- `App.CLI(...)`
- `App.Serve(...)`
- `App.Launch(...)`

### `core/`

定义所有核心抽象：

- `App`
- `Context`
- `Pipeline`
- `Stage`
- `TypedStage`
- `InteractionPlugin`
- `TrackerProvider`

### `pipeline/`

提供执行器实现：

- `LinearPipeline`
- `FSMPipeline`

### `stages/`

提供内置业务能力：

- `cond`
- `fan`
- `extract`
- `download`

### `cli/`

命令行 transport adapter。

### `server/`

HTTP / SSE transport adapter。

### `x/grasp/`

一个真实的 Flowkit 应用，展示如何组合 stage、pipeline、tracker 与 interaction plugin。

## 推荐阅读入口

- `app.go`
- `core/context.go`
- `core/stage.go`
- `core/pipeline.go`
- `pipeline/linear.go`
- `pipeline/fsm.go`
- `x/grasp/pipeline.go`
