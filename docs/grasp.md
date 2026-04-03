# 示例：x/grasp

`x/grasp` 是仓库内最完整的真实应用，完整演示了 Flowkit 的全部核心能力。

## 最小启动

```go
// examples/grasp/main.go
func main() {
    p := grasp.NewGraspPipeline()
    if err := p.Launch(); err != nil {
        log.Fatal(err)
    }
}
```

```bash
# CLI 模式
./grasp -url "https://www.pexels.com/search/cat/" -dest ./downloads

# HTTP/SSE 模式
./grasp serve --addr=:8080
```

---

## 功能概览

`x/grasp` 组合了：

- `extract.Stage` — 解析 URL，提取可下载资源列表
- `InteractionPlugin` — 终端选择（CLI）/ SSE 回调（Web）
- `download.Stage` — 分片并发下载，支持进度追踪
- `TrackerProvider` — mpb 进度条（CLI）/ SSE track 事件（Web）

---

## 数据流

```
Task.URLs
    │
    ▼
extract.Stage          ← 支持多轮（extract.MaxRounds）
    │ []extract.Item
    ▼
selectItems            ← Selector 函数 / InteractionPlugin
    │ []extract.Item（已选）
    ▼
buildDownloadTasks     ← TransformFunc（Item → download.Task）
    │ []*download.Task
    ▼
download.Stage         ← 分片并发，OnProgress 桥接 Tracker
    │ []*download.Result
    ▼
*Report
```

---

## 核心类型

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

    // 可选：覆盖默认行为
    Selector  SelectFunc   // func(*core.Context, []extract.Item) ([]extract.Item, error)
    Transform TransformFunc // func(*core.Context, extract.Item, *download.Opts) (*download.Task, error)
}

type ExtractConfig struct {
    MaxRounds         int    // 最大提取轮数（默认 1）
    ForcedParser      string // 强制指定 Parser 名称
    WorkerConcurrency int    // 并发提取数（默认 1）
}

type DownloadConfig struct {
    Dest               string
    Overwrite          bool
    TaskConcurrency    int
    BestEffort         bool          // true = 单任务失败不中止整批
    SegmentConcurrency int           // 每个任务的分片并发数
    ChunkSize          int64         // 分片大小（字节，0 = 自动）
    RetryInterval      time.Duration
}
```

### Report

```go
type Report struct {
    Success           bool               `json:"success"`
    Partial           bool               `json:"partial,omitempty"`
    DurationMs        int64              `json:"duration_ms"`
    Rounds            int                `json:"rounds"`
    ParsedItems       int                `json:"parsed_items"`
    Downloaded        []*download.Result `json:"downloaded"`
    DownloadSucceeded int                `json:"download_succeeded,omitempty"`
    DownloadFailed    int                `json:"download_failed,omitempty"`
    DownloadFailures  []DownloadFailure  `json:"download_failures,omitempty"`
}
```

---

## CLI 参数

**顶层参数**

| Flag | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `-url` | `[]string` | **required** | 目标 URL（可重复：`-url a -url b`） |
| `-proxy` | string | — | 代理地址 |
| `-timeout` | duration | `30s` | 请求超时 |
| `-retry` | int | `3` | 重试次数 |
| `-header` | `map[string]string` | — | 额外请求头（可重复：`-header Key=Val`） |

**提取配置（前缀 `extract.`）**

| Flag | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `-extract.rounds` | int | `1` | 最大提取轮数 |
| `-extract.parser` | string | — | 强制指定 Parser 名称 |
| `-extract.concurrency` | int | `1` | 并发提取 worker 数 |

**下载配置（前缀 `download.`）**

| Flag | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `-download.dest` | string | `.` | 下载目标目录 |
| `-download.overwrite` | bool | `false` | 覆盖已存在文件 |
| `-download.concurrency` | int | `0` | 并发下载任务数（0 = 不限） |
| `-download.best-effort` | bool | `false` | 单任务失败不中止整批 |
| `-download.segments` | int | `3` | 每任务分片并发数 |
| `-download.chunk-size` | int64 | `0` | 分片大小（0 = 自动） |
| `-download.retry-interval` | duration | — | 重试间隔 |

**示例**

```bash
./grasp \
  -url "https://www.pexels.com/search/cat/" \
  -timeout 60s \
  -header "Authorization=Bearer token" \
  -extract.rounds 2 \
  -download.dest ./downloads \
  -download.concurrency 4 \
  -download.best-effort
```

---

## Pipeline 结构

```go
type Pipeline struct {
    flowkit.App[*Task, *Report]
    *pipeline.LinearPipeline
    extractor         *extract.Stage
    downloader        *download.Stage
    defaultSelector   SelectFunc
    defaultTransform  TransformFunc
    interactionPlugin core.InteractionPlugin
    trackerProvider   core.TrackerProvider
}
```

通过 `Option` 自定义默认行为：

```go
p := grasp.NewGraspPipeline(
    grasp.WithDefaultSelector(grasp.SelectAll),
    grasp.WithDefaultTransform(myTransform),
    grasp.WithTrackerProvider(myTracker),
    grasp.WithInteractionPlugin(myPlugin),
)
```

---

## 扩展 Parser

`x/grasp` 默认只注册了 `pexels.APIParser`。添加自定义站点解析器：

```go
extractor := extract.NewStage("extractor")
extractor.Mount(&MyParser{})  // 实现 extract.Parser 接口
```

或通过 `Option`：

```go
p := grasp.NewGraspPipeline(
    grasp.WithParsers(&MyParser{}),
)
```

Parser 接口：

```go
type Parser interface {
    Name() string
    Match(url string) bool
    Parse(ctx *core.Context, task *Task) ([]Item, error)
}
```

---

## 交互机制

选择阶段（`selectItems`）优先级：

1. `task.Selector != nil` → 直接调用，跳过交互
2. `rc.Runtime.InteractionPlugin != nil` → 调用插件（SSE 场景）
3. `p.interactionPlugin != nil` → 调用默认插件（CLI 场景）
4. 以上都无 → 调用 `p.defaultSelector`（兜底，默认 `SelectAll`）

CLI 交互实现（`x/grasp/interact.go`）：终端展示列表，读取用户输入的序号。

SSE 交互：推送 `interact` 事件，前端展示 UI 后通过 `/answer` 回传选择结果。
