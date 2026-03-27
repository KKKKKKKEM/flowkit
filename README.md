# FlowKit

FlowKit 是一个以 **Stage（阶段）** 为核心执行单元的 Go 工作流框架，支持线性（Linear）和有限状态机（FSM）两种编排模式，内置中间件系统、进度上报、人机交互和 SSE 服务层。

## 特性

- **多种编排模式**：Linear（顺序）、FSM（状态机驱动）
- **中间件系统**：洋葱模型，内置日志、重试、超时、Panic 恢复、指标采集
- **内置 Stage 组件**：条件跳转（cond）、并行 Fan-out（fan）
- **内置业务 Stage**：HTTP 下载（支持分块/断点续传）、内容解析（URL 正则匹配）
- **双模式交互**：CLI stdin 阻塞 / Web SSE Suspend-Resume 协议，接口统一
- **实时进度上报**：CLI 终端进度条（mpb）/ SSE 事件推送，接口统一
- **高级封装 x/grasp**：开箱即用的网页抓取 Pipeline（提取 → 选择 → 下载）

## 安装

```bash
go get github.com/KKKKKKKEM/flowkit
```

需要 Go 1.25+。

## 快速示例

### 线性 Pipeline

```go
lp := pipeline.NewLinearPipeline()
lp.Register(&FetchStage{}, &ParseStage{}, &SaveStage{})
lp.Use(
    middleware.RecoveryMiddleware(logger),
    middleware.LoggingMiddleware(logger),
    middleware.RetryMiddleware(&core.DefaultErrorPolicy{MaxRetries: 3}),
)

rc := core.NewRunContext(context.Background(), "trace-001")
rc.WithValue("url", "https://example.com/data")

report, err := lp.Run(rc, "fetch")
```

### FSM Pipeline（条件跳转）

```go
router := cond.New("router",
    cond.WithBranch(func(rc *core.RunContext) bool {
        return rc.Values["status"] == "retry"
    }, "retry-stage"),
    cond.WithFallback("finalize"),
)

fp := pipeline.NewFSMPipeline()
fp.Register(&CheckStage{}, router, &RetryStage{}, &FinalizeStage{})
report, err := fp.Run(rc, "check")
```

### 网页抓取（x/grasp）

```go
// CLI 模式：终端交互选择 + 进度条
p := grasp.NewGraspPipeline(
    grasp.WithExtractor(extractor),
    grasp.WithDownloader(download.NewStage("download")),
    grasp.WithPlugin(grasp.CLISelectPlugin{}),
    grasp.WithProgress(grasp.NewMpbReporter()),
)
report, err := p.Invoke(context.Background(), task)

// Web 模式：REST + SSE 实时推送
p.Serve(":8080") // 自动挂载 POST /run 和 POST /run/answer
```

## 项目结构

```
flowkit/
├── core/          # 核心抽象：Stage、RunContext、Middleware、App
├── pipeline/      # 执行引擎：Linear、FSM
├── middleware/    # 内置中间件：Logging、Retry、Timeout、Recovery、Metrics
├── stages/
│   ├── cond/      # 条件跳转 Stage
│   └── fan/       # 并行 Fan-out Stage
├── builtin/
│   ├── download/  # HTTP 下载 Stage
│   ├── extract/   # 内容解析 Stage
│   └── serve/     # Gin + SSE 服务层
├── x/grasp/       # 高级封装：网页抓取 Pipeline
│   └── sites/     # 站点解析器（pexels 示例）
└── examples/      # 完整示例
    ├── grasp-pexels/        # CLI 模式抓取 Pexels 图片
    └── grasp-pexels-server/ # Web/SSE 模式抓取 Pexels 图片
```

## 核心概念

### Stage — 最小执行单元

```go
type Stage interface {
    Name() string
    Run(rc *RunContext) StageResult
}
```

每个 Stage 接收 `RunContext`（携带共享状态），返回 `StageResult`（含状态、输出、指标、下一跳）。

### RunContext — 贯穿全局的上下文

实现 `context.Context` 接口，同时提供 `Values`（共享键值存储）、TraceID、进度上报器、交互挂起函数等业务能力。

### Middleware — 洋葱模型

```go
lp.Use(
    middleware.RecoveryMiddleware(logger),  // 最外层
    middleware.LoggingMiddleware(logger),
    middleware.TimeoutMiddleware(30 * time.Second),
    middleware.RetryMiddleware(policy),     // 最内层
)
```

### SSE 交互协议

Pipeline 执行过程中需要用户决策时，通过 `rc.Suspend(interaction)` 挂起，框架向 SSE 流发送 `interact` 事件，等待客户端通过 `POST /answer` 提交答案后继续执行。CLI 和 Web 模式共用同一 `InteractionPlugin` 接口。

## 文档

| 文档 | 说明 |
|------|------|
| [架构文档](docs/architecture.md) | 整体架构、核心概念、数据流、模块关系 |
| [API 参考](docs/api-reference.md) | 所有包的接口签名和类型定义 |
| [使用指南](docs/guide.md) | 常见场景代码示例和最佳实践 |

## 示例

### 运行 Pexels CLI 示例

```bash
# 需要设置 Authorization Header（Pexels API Key）
go run examples/grasp-pexels/main.go
```

### 运行 Pexels Web 示例

```bash
go run examples/grasp-pexels-server/main.go
# 服务启动在 :8080
# POST /run  — 启动抓取，返回 SSE 流
# POST /run/answer — 提交选择答案
```

## 依赖

| 依赖 | 版本 | 用途 |
|------|------|------|
| `gin-gonic/gin` | v1.9.1 | HTTP 服务框架 |
| `google/uuid` | v1.6.0 | TraceID / Session ID 生成 |
| `tidwall/gjson` | v1.18.0 | 高性能 JSON 解析 |
| `vbauerster/mpb` | v8.12.0 | CLI 终端进度条 |

## License

MIT
