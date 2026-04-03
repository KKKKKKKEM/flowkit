# 内置 Stages

## cond.Stage — 条件跳转

专为 `FSMPipeline` 设计，顺序评估条件分支，第一个满足的分支胜出。

```go
import "github.com/KKKKKKKEM/flowkit/stages/cond"

router := cond.New("router",
    cond.WithBranch(func(rc *core.Context) bool {
        items, ok := core.GetState[[]extract.Item](rc, "items")
        return ok && len(items) > 0
    }, "select"),
    cond.WithBranch(func(rc *core.Context) bool {
        _, ok := core.GetState[string](rc, "error")
        return ok
    }, "handle-error"),
    cond.WithFallback("no-result"),  // 所有分支都不命中时的兜底
)
```

**执行逻辑：**
1. 按 `WithBranch` 注册顺序依次调用 `When(rc)`
2. 第一个返回 `true` 的分支 → `StageResult{Next: branch.Next}`
3. 无分支命中 → `StageResult{Next: fallback}`；`fallback == ""` 表示流程结束

**SharedState 输入/输出：** 不读写 SharedState，仅读取 Context 做判断。

---

## fan.Stage — 并发 Fan-out

将多个子 Stage 并发执行，支持三种等待/失败策略。

```go
import "github.com/KKKKKKKEM/flowkit/stages/fan"

fanStage := fan.New(
    "parallel-fetch",          // Stage 名称
    "merge-results",           // 成功后的 Next Stage（FSM 用）
    []core.Stage{stageA, stageB, stageC},
    fan.WithWaitStrategy(fan.WaitAll),          // WaitAll（默认）/ WaitAny
    fan.WithFailStrategy(fan.FailFast),         // FailFast（默认）/ BestEffort
    fan.WithConflictStrategy(fan.OverwriteOnConflict), // 输出键冲突策略
)
```

### 等待策略（WaitStrategy）

| 策略 | 行为 |
|------|------|
| `WaitAll`（默认） | 等所有子 Stage 完成，合并所有 `Outputs` |
| `WaitAny` | 第一个成功即返回，取消其余并发子 Stage；全部失败才返回 `failed` |

### 失败策略（FailStrategy，仅 WaitAll 模式）

| 策略 | 行为 |
|------|------|
| `FailFast`（默认） | 任一子 Stage 失败 → 立即取消其余，整体返回 `failed` |
| `BestEffort` | 允许部分失败，只要至少一个成功即整体成功；合并成功子 Stage 的 `Outputs` |

### 输出合并（ConflictStrategy）

| 策略 | 行为 |
|------|------|
| `OverwriteOnConflict`（默认） | 后写的键覆盖先写的 |
| `ErrorOnConflict` | 键冲突时整体返回 `failed` |

**注意：** 每个子 Stage 使用 `rc.WithContext(cancelCtx)` 运行，取消信号可传播；但 SharedState **不隔离**（所有子 Stage 共享父级 `rc`）。若需隔离，在子 Stage 内自行 `rc.Fork()`。

---

## extract.Stage — 内容提取

URL 匹配 → Parser 选择 → 解析出 `[]Item`。

```go
import "github.com/KKKKKKKEM/flowkit/stages/extract"

stage := extract.NewStage("extract",
    extract.WithNextStage("select"),
)
stage.Mount(myExtractor1, myExtractor2)
```

### 数据流

| SharedState 键 | 类型 | 方向 |
|----------------|------|------|
| `"task"` | `*extract.Task` | 输入（由调用方提前写入） |
| `"items"` | `[]extract.Item` | 输出 |

### Task 结构

```go
type Task struct {
    URL          string
    ForcedParser string   // 非空时跳过 URL 匹配，直接使用指定 Hint 的 Parser
    MaxRounds    int      // 多轮解析深度上限
    Opts         *Opts    // Proxy / Timeout / Retry / Headers
    OnItems      func(round int, items []Item)  // 每轮解析完成回调
}
```

### Parser 选择逻辑

```
ForcedParser != ""
  → 遍历所有 Extractor，找 Hint == ForcedParser 的 Parser
  → 找不到 → 报错

ForcedParser == ""
  → 所有 Parser.Pattern.MatchString(URL) 为 true 的候选
  → 按 Priority 降序排列
  → 取第一个（最高优先级）
```

### Extractor 接口

```go
type Extractor interface {
    Name() string
    Handlers() []*Parser
}

type Parser struct {
    Pattern  *regexp.Regexp  // URL 匹配正则
    Priority int             // 越大优先级越高
    Hint     string          // 语义标注（"search"、"detail" 等）
    Parse    func(ctx context.Context, task *Task, opts *Opts) ([]Item, error)
}
```

### Item 结构

```go
type Item struct {
    Name     string
    URI      string          // 直链 or 需继续解析的页面 URI
    IsDirect bool            // true = 可直接下载
    Meta     map[string]any  // 封面、时长、分辨率等业务元数据
}
```

---

## download.Stage — 批量下载

从 SharedState 读取 `[]*Task`，并发下载，写回 `[]*Result`。

```go
import "github.com/KKKKKKKEM/flowkit/stages/download"

stage := download.NewStage("download",
    download.WithMaxConcurrency(4),
    download.WithFailStrategy(download.BatchFailFast),  // 默认
    download.WithNextStage("done"),
)
stage.Register(myHTTPDownloader)
```

### 数据流

| SharedState 键 | 类型 | 方向 |
|----------------|------|------|
| `"tasks"` | `[]*download.Task` | 输入 |
| `"results"` | `[]*download.Result` | 输出 |

### Task 结构

```go
type Task struct {
    URI        string
    Opts       *Opts            // Dest / Proxy / Timeout / Retry / Concurrency / ChunkSize
    OnProgress ProgressFunc     // func(downloaded, total int64)
    OnComplete CompleteFunc     // func(result *Result)
    OnError    ErrorFunc        // func(err error)
    Meta       map[string]any   // 协议专属：method / headers / body（HTTP 用）
}
```

### Downloader 接口

```go
type Downloader interface {
    Name() string
    CanHandle(task *Task) bool                              // 根据 URI scheme 或 Meta 判断
    Download(ctx context.Context, task *Task) (*Result, error)
}
```

`Register(d Downloader)` 后注册的 Downloader 优先级更高（前插入 slice）。`Dispatch` 返回第一个 `CanHandle` 为 `true` 的 Downloader。

### 失败策略

| 策略 | 行为 |
|------|------|
| `BatchFailFast`（默认） | 第一个失败立即取消其余，返回 `nil, &BatchError` |
| `BatchBestEffort` | 所有任务都运行完，部分失败也返回已成功的 `[]*Result`，`err` 为 `*BatchError` |

`BatchError` 实现 `Unwrap() []error`，可用 `errors.As` 提取单个失败原因。

### 分片下载（Segments）

`Opts.Concurrency > 1` 时，由 Downloader 自行决定是否分片（需协议支持 Range 请求）。`Segment` 结构记录每片的起止字节和已写字节，支持断点续传状态恢复。
