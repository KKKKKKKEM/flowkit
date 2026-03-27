# FlowKit 架构文档

## 概述

FlowKit 是一个以 **Stage（阶段）** 为核心执行单元、支持多种编排模式的 Go 工作流框架。它将业务逻辑拆分为可组合的 Stage，通过 Pipeline 驱动执行，并提供中间件系统、错误策略、进度上报和人机交互等能力。

框架模块路径：`github.com/KKKKKKKEM/flowkit`

---

## 目录结构

```
flowkit/
├── core/                   # 框架核心抽象（接口、类型、上下文）
│   ├── app.go              # App 接口：可被传输层调用的执行单元
│   ├── context.go          # RunContext：贯穿整个执行流的上下文对象
│   ├── stage.go            # Stage 接口、StageResult、ErrorPolicy
│   ├── middleware.go       # Middleware 类型和 Chain 组合器
│   ├── interaction.go      # 人机交互抽象（Suspend/Resume 协议）
│   └── reporter.go         # 进度上报接口
│
├── pipeline/               # Pipeline 执行引擎
│   ├── pipeline.go         # Pipeline 接口、RunReport、执行模式定义
│   ├── linear.go           # LinearPipeline：顺序执行
│   └── fsm.go              # FSMPipeline：有限状态机驱动
│
├── middleware/             # 开箱即用的中间件
│   ├── logging.go          # 日志记录
│   ├── metrics.go          # 指标采集
│   ├── recovery.go         # Panic 恢复
│   ├── retry.go            # 自动重试
│   └── timeout.go          # 超时控制
│
├── stages/                 # 通用 Stage 组件
│   ├── cond/               # 条件跳转 Stage（FSM 专用）
│   └── fan/                # 并行 Fan-out Stage
│
├── builtin/                # 内置业务 Stage
│   ├── download/           # HTTP 下载 Stage（支持分块、断点续传）
│   ├── extract/            # 内容解析 Stage（URL 匹配 → Parser）
│   └── serve/              # HTTP 服务层（Gin + SSE）
│
├── x/grasp/                # 高级封装：网页抓取 Pipeline
│   ├── pipeline.go         # GraspPipeline：extract + select + download 三段式
│   ├── task.go             # Task 定义（抓取任务配置）
│   ├── select.go           # 选择函数（SelectAll / SelectFirst / SelectByIndices）
│   ├── interaction.go      # CLI / Web 两种交互插件实现
│   ├── reporter.go         # CLI（mpb）和 SSE 两种进度上报实现
│   ├── serve.go            # Gin 路由注册和独立启动
│   ├── option.go           # 选项函数
│   └── sites/pexels/       # Pexels 网站解析器（示例）
│
└── examples/               # 完整使用示例
    ├── grasp-pexels/       # CLI 模式抓取 Pexels 图片
    └── grasp-pexels-server/ # Web/SSE 模式抓取 Pexels 图片
```

---

## 核心概念

### 1. Stage — 最小执行单元

`Stage` 是工作流的原子操作单位，定义于 `core/stage.go`：

```go
type Stage interface {
    Name() string
    Run(rc *RunContext) StageResult
}
```

每次执行返回 `StageResult`，包含：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Status` | `StageStatus` | `success` / `skipped` / `retry` / `failed` |
| `Next` | `string` | FSM 模式下指定下一个 Stage 的名称 |
| `Outputs` | `map[string]any` | 输出数据，自动合并到 `RunContext.Values` |
| `Metrics` | `map[string]float64` | 业务指标（如下载字节数、耗时） |
| `Err` | `error` | 失败原因 |

### 2. RunContext — 贯穿全局的上下文

`RunContext` 实现 `context.Context` 接口，同时承载业务数据，是所有 Stage 之间共享状态的载体：

```go
type RunContext struct {
    ctx       context.Context
    TraceID   string
    Values    SharedState        // Stage 间共享数据（key-value）
    Tags      map[string]string
    StartedAt time.Time
}
```

**核心方法：**

- `WithValue(key, val)` — 写入共享状态
- `WithTimeout(d)` — 创建带超时的子上下文
- `WithCancel()` — 创建可取消的子上下文
- `WithSuspend(fn)` — 注入挂起函数（Web 交互模式）
- `WithReporter(r)` — 注入进度上报器
- `Value(key)` — 先查业务 Values，再回退到底层 context

### 3. Pipeline — 执行引擎

Pipeline 负责编排和驱动 Stage 的执行：

```go
type Pipeline interface {
    Mode() Mode
    Register(stages ...core.Stage) Pipeline
    Run(rc *core.RunContext, entry string) (*RunReport, error)
}
```

执行结果 `RunReport` 包含：

```go
type RunReport struct {
    Mode         Mode
    Success      bool
    TraceID      string
    StageOrder   []string                    // 实际执行顺序
    StageResults map[string]core.StageResult // 每个 Stage 的详细结果
    DurationMs   int64
}
```

### 4. Middleware — 横切关注点

中间件采用经典的洋葱模型（与 net/http 的 Handler 链同构）：

```go
type StageRunner func(rc *RunContext, st Stage) StageResult
type Middleware  func(next StageRunner) StageRunner
```

通过 `Chain(mws...)` 将多个中间件按注册顺序组合：

```go
// 执行顺序：Logging → Recovery → Retry → 实际 Stage
lp.Use(
    middleware.LoggingMiddleware(logger),
    middleware.RecoveryMiddleware(logger),
    middleware.RetryMiddleware(policy),
)
```

### 5. App — 传输层适配器

`App` 是框架暴露给传输层（HTTP、gRPC 等）的统一接口：

```go
type App[Req, Resp any] interface {
    Invoke(ctx context.Context, req Req) (Resp, error)
}
```

`AppFunc` 允许直接将函数用作 `App`，无需定义新类型。

---

## Pipeline 执行模式

### Linear 模式（线性顺序执行）

`LinearPipeline` 从 `entry` Stage 开始，每个 Stage 执行一次后结束。适用于固定步骤的处理流程。

```
entry → Stage_A → Stage_B → Stage_C → 结束
```

Stage 的 `Next` 字段在 Linear 模式下被忽略。每个 Stage 的 `Outputs` 自动合并到 `rc.Values`。

```go
lp := pipeline.NewLinearPipeline()
lp.Register(stageA, stageB, stageC)
lp.Use(middleware.LoggingMiddleware(logger))
report, err := lp.Run(rc, "stageA")
```

### FSM 模式（有限状态机）

`FSMPipeline` 通过每个 Stage 返回的 `Result.Next` 字段决定下一跳，实现动态流程控制。当 `Next` 为空时流程终止。

```
entry → Stage_A
           ├─(Next="B")→ Stage_B → 结束
           └─(Next="C")→ Stage_C → Stage_B → 结束
```

内置环路检测：单个 Stage 访问次数超过 `maxVisits`（默认 999）时报错。

```go
fp := pipeline.NewFSMPipeline()
fp.WithMaxVisits(10)
fp.Register(stageA, stageB, stageC)
report, err := fp.Run(rc, "stageA")
```

---

## 通用 Stage 组件

### cond.Stage — 条件跳转

专为 FSM 模式设计，按顺序匹配 Branch，第一个满足条件的分支胜出：

```go
router := cond.New("router",
    cond.WithBranch(func(rc *core.RunContext) bool {
        return rc.Values["status"] == "retry"
    }, "retry-stage"),
    cond.WithBranch(func(rc *core.RunContext) bool {
        return rc.Values["count"].(int) > 10
    }, "finalize"),
    cond.WithFallback("process"),
)
```

### fan.Stage — 并行 Fan-out

将多个子 Stage 并行执行，支持三种行为策略：

| 模式 | 说明 |
|------|------|
| `WaitAll` + `FailFast`（默认）| 等待全部完成，任一失败则取消其余并返回失败 |
| `WaitAll` + `BestEffort` | 等待全部完成，忽略失败，至少一个成功即可 |
| `WaitAny` | 第一个成功即返回，取消其余 |

```go
parallel := fan.New("parallel", "next-stage",
    []core.Stage{stageA, stageB, stageC},
    fan.WithWaitStrategy(fan.WaitAny),
)
```

输出通过 `ConflictStrategy` 合并：`OverwriteOnConflict`（默认）或 `ErrorOnConflict`。

---

## 内置 Stage

### builtin/download — HTTP 下载

`DirectDownloadStage` 从 `rc.Values["tasks"]` 读取下载任务，支持：

- 单文件 / 批量并行下载
- 分块下载（`Concurrency > 1`）
- 断点续传（`.meta` 文件记录分片进度）
- 代理支持（`http://`、`https://`、`socks5://`、`"env"` 自动读取环境变量）
- 重试（含可配置间隔）
- 回调：`OnProgress`、`OnComplete`、`OnError`

```go
stage := download.NewStage("download",
    download.WithFallbackOpts(download.Opts{
        Dest:        "./output",
        Concurrency: 4,
        Timeout:     30 * time.Second,
    }),
)
```

### builtin/extract — 内容解析

`extract.Stage` 注册多个 `Extractor`，运行时根据 URL 正则匹配对应 `Parser`：

```go
type Extractor interface {
    Name() string
    Handlers() []*Parser
}

type Parser struct {
    Pattern  *regexp.Regexp
    Priority int
    Hint     string
    Parse    func(ctx context.Context, task *Task, opts *Opts) ([]ParseItem, error)
}
```

支持多轮解析（`MaxRounds`）：`ParseItem.IsDirect=false` 的条目会进入下一轮解析队列。

```go
extractor := extract.NewStage("extractor")
extractor.Mount(&MyExtractor{})
```

### builtin/serve — HTTP 服务层

#### 普通 HTTP 注册

```go
serve.Register(router, "/api/run", myApp)
serve.RegisterFunc(router, "/api/run", func(ctx context.Context, req MyReq) (MyResp, error) {
    // ...
})
```

#### SSE（Server-Sent Events）模式

SSE 模式支持长时间运行的任务与实时进度推送，并实现了 **Suspend/Resume** 交互协议：

**事件类型：**

| 事件 | 说明 |
|------|------|
| `session` | 连接建立，携带 `session_id` |
| `progress` | 进度更新 |
| `interact` | Pipeline 挂起，等待客户端交互 |
| `done` | 执行完成，携带最终结果 |
| `error` | 执行失败 |

**Suspend/Resume 协议：**
1. Pipeline 调用 `rc.Suspend(interaction)` 时阻塞
2. 框架向 SSE 流推送 `pending` 事件（含 `interaction_id`）
3. 客户端收到后，通过 `POST /answer` 提交答案
4. Pipeline 解除阻塞，继续执行

---

## 人机交互系统

`InteractionPlugin` 接口统一了 CLI 和 Web 两种交互模式：

```go
type InteractionPlugin interface {
    Type() InteractionType
    Interact(rc *RunContext, i Interaction) error
}
```

框架通过 `rc.WithSuspend(fn)` 注入 `SuspendFunc`，实现模式透明切换：

- **CLI 模式**：`Interact` 阻塞在 stdin 等待用户输入
- **Web/SSE 模式**：`Interact` 调用 `rc.Suspend(i)`，阻塞直到客户端提交答案

---

## 进度上报系统

`ProgressReporter` / `ProgressTracker` 接口抽象了进度上报行为：

```go
type ProgressReporter interface {
    Track(key string, total int64) ProgressTracker
    Wait()
}

type ProgressTracker interface {
    Update(downloaded int64)
    Done()
}
```

内置两种实现：

| 实现 | 场景 |
|------|------|
| `MpbReporter` | CLI 终端，使用 `vbauerster/mpb` 渲染进度条 |
| `SSEReporter` | Web 模式，将进度以 SSE 事件推送给客户端 |

---

## 错误处理

### 错误分类

```go
const (
    ErrTransient ErrorClass = "transient"  // 可重试（网络超时）
    ErrBusiness  ErrorClass = "business"   // 不可重试（参数错误）
    ErrFatal     ErrorClass = "fatal"      // 立即终止（context 取消）
)
```

### DefaultErrorPolicy

- `context.Canceled` → `ErrFatal`（不重试）
- `context.DeadlineExceeded` → `ErrTransient`（可重试）
- 其他 → `ErrBusiness`（不重试）

### 通过中间件实现重试

```go
policy := &core.DefaultErrorPolicy{MaxRetries: 3}
lp.Use(middleware.RetryMiddleware(policy))
```

---

## x/grasp — 网页抓取 Pipeline

`x/grasp` 是基于框架核心能力构建的高级抓取工具，封装了**提取 → 选择 → 下载**三段式工作流。

### 工作流程

```
URLs
  │
  ▼
[Extract]  并发多轮解析，URL 正则匹配 Parser，输出 ParseItem 列表
  │
  ▼
[Select]   用户/程序过滤要下载的条目（SelectAll / SelectFirst / 自定义）
  │
  ▼  
[Transform] ParseItem → download.Task（注入 Opts、进度回调）
  │
  ▼
[Download]  并行 HTTP 下载，支持分块、断点续传、进度上报
  │
  ▼
Report { Downloaded, Rounds, ParsedItems, DurationMs }
```

### Task 配置

```go
task := &grasp.Task{
    URLs:    []string{"https://example.com/page"},
    Proxy:   "http://proxy:8080",
    Timeout: 30 * time.Second,
    Retry:   3,

    Extract: grasp.ExtractConfig{
        MaxRounds:   2,         // 最多两轮解析（列表页 → 详情页）
        Concurrency: 4,         // 并发解析
    },
    Download: grasp.DownloadConfig{
        Dest:        "./output",
        Concurrency: 8,         // 分块下载并发数
        Overwrite:   false,
    },

    Selector:  grasp.SelectFirst(10),  // 只下载前 10 个
    Transform: myTransformFn,          // 自定义下载参数
}
```

### 实现自定义解析器

```go
type MyExtractor struct{}

func (e *MyExtractor) Name() string { return "my-extractor" }

func (e *MyExtractor) Handlers() []*extract.Parser {
    return []*extract.Parser{
        {
            Pattern:  regexp.MustCompile(`^https://example\.com/`),
            Priority: 10,
            Hint:     "list",
            Parse: func(ctx context.Context, task *extract.Task, opts *extract.Opts) ([]extract.ParseItem, error) {
                // 解析页面，返回 ParseItem 列表
                return []extract.ParseItem{
                    {Name: "file.jpg", URI: "https://cdn.example.com/file.jpg", IsDirect: true},
                }, nil
            },
        },
    }
}
```

### CLI 模式

```go
p := grasp.NewGraspPipeline(
    grasp.WithExtractor(extractor),
    grasp.WithDownloader(downloader),
    grasp.WithPlugin(grasp.CLISelectPlugin{}),  // 终端交互选择
    grasp.WithProgress(grasp.NewMpbReporter()), // 终端进度条
)
report, err := p.Invoke(context.Background(), task)
```

### Web/SSE 模式

```go
p := grasp.NewGraspPipeline(
    grasp.WithExtractor(extractor),
    grasp.WithDownloader(downloader),
    grasp.WithPlugin(grasp.WebSelectPlugin{}), // SSE 交互选择
)
p.Serve(":8080") // 自动挂载 POST /run 和 POST /run/answer
```

---

## 数据流图

```
┌─────────────────────────────────────────────────────────┐
│                       调用方                              │
│  HTTP Client / CLI / 其他服务                             │
└───────────────┬─────────────────────────────────────────┘
                │ Req
                ▼
┌───────────────────────────────┐
│      serve 层（传输适配）       │
│  serve.Register / serve.SSE   │
│  反序列化 Req → 调用 App.Invoke │
└───────────────┬───────────────┘
                │
                ▼
┌───────────────────────────────┐
│         App.Invoke            │
│  构造 RunContext               │
│  注入 SuspendFunc（SSE 模式）  │
│  注入 ProgressReporter        │
└───────────────┬───────────────┘
                │ rc
                ▼
┌───────────────────────────────┐
│         Pipeline.Run          │
│  ┌─────────────────────────┐  │
│  │  Middleware Chain        │  │
│  │  Logging → Recovery →   │  │
│  │  Timeout → Retry        │  │
│  │       ↓                 │  │
│  │   Stage.Run(rc)         │  │
│  │       ↓                 │  │
│  │  StageResult → merge    │  │
│  │  Outputs → rc.Values    │  │
│  └─────────────────────────┘  │
│  （重复直到流程终止）            │
└───────────────┬───────────────┘
                │ RunReport
                ▼
┌───────────────────────────────┐
│      序列化 Resp               │
│      返回给调用方              │
└───────────────────────────────┘
```

---

## 依赖说明

| 依赖 | 用途 |
|------|------|
| `github.com/gin-gonic/gin` | HTTP 服务框架（serve 层） |
| `github.com/google/uuid` | 生成 TraceID 和 Session ID |
| `github.com/tidwall/gjson` | 高性能 JSON 解析（用于内容提取） |
| `github.com/vbauerster/mpb/v8` | CLI 终端进度条 |
