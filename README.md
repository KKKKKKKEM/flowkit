# Flowkit

面向 Go 的轻量工作流框架。把任意业务逻辑包装成可组合的 Stage → Pipeline → App，同时跑在 CLI 和 HTTP/SSE 两种入口上。

```
import "github.com/KKKKKKKEM/flowkit"
```

## 为什么需要 Flowkit

很多业务流程本质上是：**解析 → 选择 → 转换 → 下载/执行**，并在过程中需要：
- 进度反馈（终端 progressbar 或 Web 实时推送）
- 人工介入（终端提示输入或 Web 弹窗等待回调）
- 同一套逻辑跑在 CLI 和 HTTP 两种场景

Flowkit 的核心价值是：**把传输层和业务逻辑彻底解耦**，让你写一遍 `App.Invoke`，就能同时暴露为 CLI 工具和 HTTP 服务。

## 核心抽象

```
App[Req, Resp]
  └── Invoke(ctx *core.Context, req Req) (Resp, error)

core.Context
  ├── context.Context（取消、超时）
  ├── *SharedState（stage 间共享数据，Set/Get/Merge）
  └── *Runtime（TraceID、TrackerProvider、InteractionPlugin）

Pipeline
  ├── LinearPipeline  — 按注册顺序执行
  └── FSMPipeline     — 按 StageResult.Next 跳转

Stage
  ├── Stage           — Run(rc *Context) StageResult
  └── TypedStage[In, Out] — Exec(rc *Context, in In) (TypedResult[Out], error)
```

## 快速上手

**AutoFlags**（推荐）— struct tag 自动映射为 CLI flag，无需手写 `flag.FlagSet`：

```go
package main

import (
    "log"
    "time"
    "github.com/KKKKKKKEM/flowkit"
    "github.com/KKKKKKKEM/flowkit/core"
)

type Req struct {
    URL     string        `cli:"url,required,usage=target URL"`
    Timeout time.Duration `cli:"timeout,default=30s,usage=request timeout"`
}
type Resp struct{ Title string }

func main() {
    app := flowkit.NewApp(func(ctx *core.Context, req *Req) (*Resp, error) {
        return &Resp{Title: "hello from " + req.URL}, nil
    })

    if err := app.Launch(
        flowkit.WithLaunchCLIOptions(
            flowkit.WithCLIAutoFlags[*Req, *Resp](),
        ),
    ); err != nil {
        log.Fatal(err)
    }
}
```

```bash
./app run -url https://example.com -timeout 60s
./app serve --addr=:8080
./app help          # 打印用法 + 全量 flag 列表
./app -h            # 等价于 help（AutoFlags 模式）
```

<details>
<summary>手动 Builder 写法</summary>

```go
flowkit.WithCLIBuilder[*Req, *Resp](func(args []string) (*Req, error) {
    fs := flag.NewFlagSet("app", flag.ContinueOnError)
    url := fs.String("url", "", "target URL (required)")
    if err := fs.Parse(args); err != nil {
        return nil, err
    }
    if *url == "" {
        return nil, fmt.Errorf("-url is required")
    }
    return &Req{URL: *url}, nil
})
```

</details>

## 项目结构

```
app.go          — 顶层门面：NewApp / Launch / CLI / Serve
core/           — 核心接口：App / Context / Pipeline / Stage / Middleware
                  InteractionPlugin / TrackerProvider
pipeline/       — 执行器实现
  linear.go     — 顺序执行，支持 Middleware
  fsm.go        — FSM 跳转，内置防环，支持 Middleware
server/         — HTTP / SSE transport
  http.go       — 普通 request/response
  sse.go        — 会话式 SSE（流式事件 + 交互回传）
  sse/          — Session、TrackerProvider、InteractionPlugin 实现
stages/         — 内置 stage
  cond/         — 条件跳转（FSM 用）
  fan/          — 并发 fan-out（wait-all / wait-any / best-effort）
  extract/      — 内容提取（Parser 注册 + URL 匹配）
  download/     — 批量下载（分片 + 并发 + 失败策略）
x/grasp/        — 完整应用示例（extract + download + CLI + SSE）
examples/grasp/ — grasp 最小启动入口
```

## 启动方式

| 方式 | 方法 | 场景 |
|------|------|------|
| 统一入口 | `app.Launch(...)` | **推荐**。按 args 自动分发 CLI / HTTP |
| 纯 CLI | `app.CLI(...)` | 只需要命令行工具 |
| HTTP + SSE | `app.Serve(addr, ...)` | 只需要服务端（默认 SSE，支持交互） |

默认支持以下子命令：

| 子命令 | 说明 |
|--------|------|
| `run [flags]` | CLI 模式（等价于裸传 flags） |
| `serve [--addr=:8080]` | 启动 HTTP/SSE 服务 |
| `help` | 打印用法说明 + flag 列表 |

```bash
./app run -url https://example.com     # CLI 模式
./app -url https://example.com         # 等价于 run
./app serve --addr=:8080               # HTTP/SSE 模式
./app help                             # 查看帮助
./app -h                               # 同上（AutoFlags 模式）
```

## 内置 Stage 一览

| Stage | 包 | 功能 |
|-------|----|------|
| `cond.Stage` | `stages/cond` | 条件跳转，顺序评估分支，配合 FSM 使用 |
| `fan.Stage` | `stages/fan` | 并发 fan-out，支持 wait-all/wait-any/best-effort |
| `extract.Stage` | `stages/extract` | 内容提取，URL 匹配 → Parser → `[]Item` |
| `download.Stage` | `stages/download` | 批量下载，分片并发，FailFast / BestEffort |

## 参考实现：x/grasp

`x/grasp` 是仓库内的完整应用，展示了：
- `extract.Stage` + `download.Stage` 的完整组合
- CLI 交互（终端选择）和 SSE 交互（Web 回调）共存
- `TrackerProvider`（mpb 进度条）和 `InteractionPlugin` 的实际接入
- `SelectFunc` / `TransformFunc` 的扩展点

```go
// examples/grasp/main.go
func main() {
    p := grasp.NewGraspPipeline()
    if err := p.Launch(); err != nil {
        log.Fatal(err)
    }
}
```

## 文档

| 文档 | 内容 |
|------|------|
| [架构概览](./docs/architecture.md) | 层次模型、数据流、模块职责 |
| [核心概念](./docs/core-concepts.md) | Context / Stage / Pipeline 接口详解 |
| [启动方式](./docs/launch-and-runtime.md) | Launch / CLI / Serve 配置参考 |
| [Pipeline 模式](./docs/pipelines.md) | Linear / FSM / Middleware |
| [内置 Stages](./docs/stages.md) | cond / fan / extract / download |
| [HTTP/SSE 模型](./docs/server-and-sse.md) | SSE 协议、事件类型、交互流程 |
| [示例：x/grasp](./docs/grasp.md) | grasp 完整解析 |
| [扩展指南](./docs/extending-flowkit.md) | 自定义 Parser / Downloader / Plugin |

## 依赖

- Go 1.22+（`go.mod` 暂写 `1.25`，实际可降版本）
- [gin](https://github.com/gin-gonic/gin) — HTTP 路由
- [mpb](https://github.com/vbauerster/mpb) — CLI 进度条（仅 x/grasp）
- [uuid](https://github.com/google/uuid) — Trace ID 生成
