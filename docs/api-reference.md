# API 参考

## core 包

### RunContext

贯穿整个执行流的上下文对象，实现 `context.Context` 接口。

```go
func NewRunContext(ctx context.Context, traceID string) *RunContext
```

**方法：**

| 方法 | 签名 | 说明 |
|------|------|------|
| `WithValue` | `(key string, val any) *RunContext` | 写入共享状态，返回自身（链式调用） |
| `WithTimeout` | `(d time.Duration) (*RunContext, context.CancelFunc)` | 创建带超时的子上下文 |
| `WithCancel` | `() (*RunContext, context.CancelFunc)` | 创建可取消的子上下文 |
| `WithSuspend` | `(fn SuspendFunc)` | 注入 SSE 挂起函数（框架内部调用） |
| `WithReporter` | `(r ProgressReporter)` | 注入进度上报器 |
| `Suspend` | `() SuspendFunc` | 获取挂起函数（可能为 nil） |
| `Reporter` | `() ProgressReporter` | 获取进度上报器（可能为 nil） |
| `Duration` | `() time.Duration` | 返回从创建到现在的耗时 |

**字段（公开）：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `TraceID` | `string` | 追踪 ID（贯穿整个 Pipeline） |
| `Values` | `SharedState` | 共享键值存储（`map[string]any`） |
| `Tags` | `map[string]string` | 元标签（标注、路由等） |
| `StartedAt` | `time.Time` | 创建时间 |

---

### Stage

```go
type Stage interface {
    Name() string
    Run(rc *RunContext) StageResult
}
```

**StageResult：**

```go
type StageResult struct {
    Status  StageStatus        // "success" | "skipped" | "retry" | "failed"
    Next    string             // FSM 模式：下一个 Stage 名称
    Outputs map[string]any     // 输出数据，自动合并到 rc.Values
    Metrics map[string]float64 // 业务指标
    Err     error              // 失败错误
}

func (sr *StageResult) IsSuccess() bool
func (sr *StageResult) IsFailed() bool
func (sr *StageResult) IsTerminal() bool
```

---

### Middleware

```go
type StageRunner func(rc *RunContext, st Stage) StageResult
type Middleware  func(next StageRunner) StageRunner

// 按注册顺序组合中间件（先注册先执行）
func Chain(mws ...Middleware) Middleware
```

---

### App

```go
type App[Req, Resp any] interface {
    Invoke(ctx context.Context, req Req) (Resp, error)
}

// 函数适配器
type AppFunc[Req, Resp any] func(context.Context, Req) (Resp, error)
func (f AppFunc[Req, Resp]) Invoke(ctx context.Context, req Req) (Resp, error)
```

---

### ErrorPolicy

```go
type ErrorPolicy interface {
    Classify(err error) ErrorClass
    ShouldRetry(stage string, err error, attempt int) bool
}

// 默认实现
type DefaultErrorPolicy struct {
    MaxRetries int
}
```

---

### InteractionPlugin

```go
type InteractionPlugin interface {
    Type() InteractionType
    Interact(rc *RunContext, i Interaction) error
}

type Interaction struct {
    Type    InteractionType
    Payload any
}

type InteractionResult struct {
    Answer any
}

// SuspendFunc 由 SSE 框架层注入到 RunContext
type SuspendFunc func(i Interaction) (InteractionResult, error)
```

---

### ProgressReporter / ProgressTracker

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

---

## pipeline 包

### Pipeline 接口

```go
type Pipeline interface {
    Mode() Mode
    Register(stages ...core.Stage) Pipeline
    Run(rc *core.RunContext, entry string) (*RunReport, error)
}

type Mode string

const (
    ModeLinear   Mode = "linear"
    ModeFSM      Mode = "fsm"
    ModeParallel Mode = "parallel"
    ModeDAG      Mode = "dag"
)
```

### LinearPipeline

```go
func NewLinearPipeline() *LinearPipeline

func (lp *LinearPipeline) Register(stages ...core.Stage) Pipeline
func (lp *LinearPipeline) Use(mw ...core.Middleware) *LinearPipeline
func (lp *LinearPipeline) Run(rc *core.RunContext, entry string) (*RunReport, error)
```

### FSMPipeline

```go
func NewFSMPipeline() *FSMPipeline

func (fp *FSMPipeline) Register(stages ...core.Stage) Pipeline
func (fp *FSMPipeline) Use(mw ...core.Middleware) *FSMPipeline
func (fp *FSMPipeline) WithMaxVisits(max int) *FSMPipeline
func (fp *FSMPipeline) Run(rc *core.RunContext, entry string) (*RunReport, error)
```

---

## middleware 包

### LoggingMiddleware

记录每个 Stage 的开始、结束、耗时和错误，并将 `duration_ms` 写入 `StageResult.Metrics`。

```go
func LoggingMiddleware(logger *log.Logger) core.Middleware
```

### RecoveryMiddleware

捕获 Stage 执行过程中的 panic，转换为 `StageFailed` 结果，防止整个程序崩溃。

```go
func RecoveryMiddleware(logger *log.Logger) core.Middleware
```

### RetryMiddleware

根据 `ErrorPolicy` 自动重试失败的 Stage。

```go
func RetryMiddleware(policy core.ErrorPolicy) core.Middleware
```

### TimeoutMiddleware

为每个 Stage 添加独立超时，超时后取消子上下文并返回失败。

```go
func TimeoutMiddleware(timeout time.Duration) core.Middleware
```

### MetricsMiddleware

统计每个 Stage 的调用次数（`total_calls`）、成功次数（`total_success`）和失败次数（`total_failures`）。

```go
func MetricsMiddleware(metricsCollector map[string]map[string]float64) core.Middleware
```

---

## stages/cond 包

条件跳转 Stage，专为 FSM 模式设计。

```go
func New(name string, opts ...Option) *Stage

// 选项
func WithBranch(when CondFunc, next string) Option
func WithFallback(next string) Option

type CondFunc func(rc *core.RunContext) bool
```

**示例：**
```go
router := cond.New("router",
    cond.WithBranch(func(rc *core.RunContext) bool {
        return rc.Values["retry"].(bool)
    }, "retry-handler"),
    cond.WithFallback("success-handler"),
)
```

---

## stages/fan 包

并行 Fan-out Stage，将多个子 Stage 并行执行。

```go
func New(name string, next string, children []core.Stage, opts ...Option) *Stage

// 等待策略
const WaitAll WaitStrategy = iota  // 等待全部完成（默认）
const WaitAny WaitStrategy = iota  // 等待第一个成功

// 失败策略
const FailFast   FailStrategy = iota  // 任一失败则停止（默认）
const BestEffort FailStrategy = iota  // 容忍失败，至少一个成功

// 冲突策略
const OverwriteOnConflict ConflictStrategy = iota  // 覆盖（默认）
const ErrorOnConflict     ConflictStrategy = iota  // 报错

// 选项
func WithWaitStrategy(s WaitStrategy) Option
func WithFailStrategy(s FailStrategy) Option
func WithConflictStrategy(s ConflictStrategy) Option
```

---

## builtin/download 包

### DirectDownloadStage

```go
func NewStage(name string, options ...Option) *DirectDownloadStage
```

**从 rc.Values 读取：**
- `rc.Values["tasks"]` — `*Task` 或 `[]*Task`

**输出到 rc.Values：**
- `rc.Values["download_results"]` — `[]*Result`

### Task / Opts

```go
type Opts struct {
    Dest          string
    Proxy         string        // "" | "http://..." | "socks5://..." | "env"
    Timeout       time.Duration
    Retry         int
    RetryInterval time.Duration
    Overwrite     bool
    Concurrency   int           // >1 时启用分块下载
    ChunkSize     int64         // 分块大小，0 = 1MB
}

type Task struct {
    *Opts
    Request    *http.Request
    OnProgress ProgressFunc  // func(downloaded, total int64)
    OnComplete CompleteFunc  // func(result *Result)
    OnError    ErrorFunc     // func(err error)
    Meta       map[string]any
}

func NewTaskFromURI(uri string, opts *Opts, headers map[string]string) (*Task, error)

type Result struct {
    Path string
    Size int64
}
```

---

## builtin/extract 包

### Stage

```go
func NewStage(name string, options ...Option) *Stage

func (s *Stage) Mount(extractors ...Extractor) *Stage
```

**从 rc.Values 读取：**
- `rc.Values["task"]` — `*Task`

**输出到 rc.Values：**
- `rc.Values["items"]` — `[]ParseItem`

### Extractor / Parser

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

type ParseItem struct {
    Name     string
    URI      string
    IsDirect bool           // false = 继续解析；true = 直接下载
    Meta     map[string]any
}
```

---

## builtin/serve 包

### 辅助函数

```go
// Func 将普通函数包装为 core.App，自动推断泛型参数，省去手写 AppFunc[Req,Resp] 转换。
func Func[Req, Resp any](fn func(context.Context, Req) (Resp, error)) core.App[Req, Resp]
```

### HTTP 注册

```go
type HTTPConfig[Req, Resp any] struct {
    App      core.App[Req, Resp]
    BuildReq func(*gin.Context) (Req, error) // 默认 ShouldBindJSON
    OnStart  func(*gin.Context, *core.RunContext, Req) // 可选，用于注入 RunContext 数据
}

func HTTP[Req, Resp any](r gin.IRouter, path string, cfg HTTPConfig[Req, Resp])
```

注册 `POST {path}`，将请求解析后调用 `cfg.App.Invoke`，结果以 JSON 响应。

### SSE 注册

```go
type SSEConfig[Req, Resp any] struct {
    App      core.App[Req, Resp]
    Store    *SSESessionStore               // 默认 DefaultSSESessionStore()
    BuildReq func(*gin.Context) (Req, error) // 默认 ShouldBindJSON
    OnStart  func(*SSESession, *core.RunContext, Req) // 可选，用于注入 Reporter 等
}

func SSE[Req, Resp any](r gin.IRouter, path string, cfg SSEConfig[Req, Resp])
```

自动注册两个路由：
- `POST {path}` — 启动执行，返回 SSE 流
- `POST {path}/answer` — 提交交互答案

### SSESessionStore

```go
func NewSSESessionStore(ttl time.Duration) *SSESessionStore
func DefaultSSESessionStore() *SSESessionStore  // TTL = 30 分钟

func (s *SSESessionStore) Create(sessionID string) *SSESession
func (s *SSESessionStore) Get(sessionID string) (*SSESession, bool)
func (s *SSESessionStore) Delete(sessionID string)
```

### SSESession

```go
func (s *SSESession) EmitProgress(data any)
```

---

## x/grasp 包

### GraspPipeline

```go
func NewGraspPipeline(opts ...Option) *Pipeline

func (p *Pipeline) Invoke(ctx context.Context, task *Task) (*Report, error)
func (p *Pipeline) GinRegister(r gin.IRouter) gin.IRouter
func (p *Pipeline) Serve(addr string) error
```

### Task

```go
type Task struct {
    URLs    []string
    Proxy   string
    Timeout time.Duration
    Retry   int
    Headers map[string]string

    Extract  ExtractConfig
    Download DownloadConfig

    Selector  SelectFunc    // 覆盖 Pipeline 默认选择器
    Transform TransformFunc // 覆盖 Pipeline 默认转换函数
}

type ExtractConfig struct {
    MaxRounds    int
    ForcedParser string  // 跳过 URL 匹配，直接使用指定 Hint
    Concurrency  int
}

type DownloadConfig struct {
    Dest          string
    Overwrite     bool
    Concurrency   int
    ChunkSize     int64
    RetryInterval time.Duration
}
```

### 选项函数

```go
func WithExtractor(e *extract.Stage) Option
func WithDownloader(d *download.DirectDownloadStage) Option
func WithSelector(fn SelectFunc) Option
func WithTransform(fn TransformFunc) Option
func WithProgress(r core.ProgressReporter) Option
func WithPlugin(plugin core.InteractionPlugin) Option
```

### 内置选择函数

```go
type SelectFunc func(ctx context.Context, items []extract.ParseItem) ([]extract.ParseItem, error)

func SelectAll(_ context.Context, items []extract.ParseItem) ([]extract.ParseItem, error)
func SelectFirst(n int) SelectFunc
func SelectByIndices(indices []int) SelectFunc
```

### 内置转换函数

```go
type TransformFunc func(ctx context.Context, item extract.ParseItem, baseOpts *download.Opts) (*download.Task, error)

func DefaultTransform(baseOpts *download.Opts) TransformFunc
```

### 交互插件

```go
// CLI 模式：阻塞 stdin 等待用户输入
type CLISelectPlugin struct{}

// Web/SSE 模式：通过 SSE Suspend/Resume 协议与客户端交互
type WebSelectPlugin struct{}
```

### 进度上报

```go
// CLI 终端进度条（基于 mpb）
func NewMpbReporter() *MpbReporter

// SSE 进度推送
func NewSSEReporter(sess *serve.SSESession) *SSEReporter
```

### Report

```go
type Report struct {
    Success     bool               `json:"success"`
    DurationMs  int64              `json:"duration_ms"`
    Rounds      int                `json:"rounds"`
    ParsedItems int                `json:"parsed_items"`
    Downloaded  []*download.Result `json:"downloaded"`
}
```
