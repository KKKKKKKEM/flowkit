# 使用指南

## 快速开始

### 安装

```bash
go get github.com/KKKKKKKEM/flowkit
```

---

## 场景一：构建线性处理流水线

适用于固定步骤、顺序执行的业务流程。

```go
package main

import (
    "context"
    "log"

    "github.com/KKKKKKKEM/flowkit/core"
    "github.com/KKKKKKKEM/flowkit/middleware"
    "github.com/KKKKKKKEM/flowkit/pipeline"
)

// 实现 Stage 接口
type FetchStage struct{}

func (s *FetchStage) Name() string { return "fetch" }
func (s *FetchStage) Run(rc *core.RunContext) core.StageResult {
    // 从 rc.Values 读取输入
    url := rc.Values["url"].(string)

    // 执行业务逻辑...
    data := fetchData(url)

    // 输出写入 rc.Values（供后续 Stage 读取）
    return core.StageResult{
        Status:  core.StageSuccess,
        Outputs: map[string]any{"data": data},
    }
}

type ParseStage struct{}

func (s *ParseStage) Name() string { return "parse" }
func (s *ParseStage) Run(rc *core.RunContext) core.StageResult {
    data := rc.Values["data"].([]byte)
    result := parseData(data)

    return core.StageResult{
        Status:  core.StageSuccess,
        Outputs: map[string]any{"result": result},
    }
}

func main() {
    logger := log.Default()

    // 构建 Pipeline
    lp := pipeline.NewLinearPipeline()
    lp.Register(&FetchStage{}, &ParseStage{})
    lp.Use(
        middleware.RecoveryMiddleware(logger),
        middleware.LoggingMiddleware(logger),
    )

    // 创建运行上下文
    rc := core.NewRunContext(context.Background(), "trace-001")
    rc.WithValue("url", "https://example.com/data")

    // 执行
    report, err := lp.Run(rc, "fetch")
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("成功: %v, 耗时: %dms", report.Success, report.DurationMs)
    log.Printf("结果: %v", rc.Values["result"])
}
```

---

## 场景二：构建状态机流程（FSM）

适用于需要动态分支、条件跳转的复杂流程。

```go
package main

import (
    "github.com/KKKKKKKEM/flowkit/core"
    "github.com/KKKKKKKEM/flowkit/pipeline"
    "github.com/KKKKKKKEM/flowkit/stages/cond"
)

// 检查状态的条件 Stage
type CheckStage struct{}
func (s *CheckStage) Name() string { return "check" }
func (s *CheckStage) Run(rc *core.RunContext) core.StageResult {
    status := getStatus()
    return core.StageResult{
        Status:  core.StageSuccess,
        Outputs: map[string]any{"status": status},
    }
}

// 路由 Stage（使用 cond.Stage）
router := cond.New("router",
    cond.WithBranch(func(rc *core.RunContext) bool {
        return rc.Values["status"] == "error"
    }, "handle-error"),
    cond.WithBranch(func(rc *core.RunContext) bool {
        return rc.Values["status"] == "pending"
    }, "retry"),
    cond.WithFallback("finalize"),  // 默认跳转
)

// 构建 FSM Pipeline
fp := pipeline.NewFSMPipeline()
fp.WithMaxVisits(5)  // 防止无限循环
fp.Register(
    &CheckStage{},
    router,
    &HandleErrorStage{},
    &RetryStage{},
    &FinalizeStage{},
)

report, err := fp.Run(rc, "check")
```

---

## 场景三：并行执行（Fan-out）

```go
import "github.com/KKKKKKKEM/flowkit/stages/fan"

// 创建并行 Stage，等待全部完成，任一失败则停止
parallel := fan.New(
    "parallel-fetch",
    "merge",  // 完成后跳转到 merge stage
    []core.Stage{
        &FetchAPIStage{},
        &FetchDatabaseStage{},
        &FetchCacheStage{},
    },
    // 可选策略
    fan.WithWaitStrategy(fan.WaitAll),
    fan.WithFailStrategy(fan.BestEffort),        // 容忍失败
    fan.WithConflictStrategy(fan.ErrorOnConflict), // 输出键冲突时报错
)

fp := pipeline.NewFSMPipeline()
fp.Register(parallel, &MergeStage{})
report, err := fp.Run(rc, "parallel-fetch")
```

---

## 场景四：HTTP 服务接入

将 Pipeline 封装为 HTTP API：

```go
import (
    "github.com/KKKKKKKEM/flowkit/builtin/serve"
    "github.com/KKKKKKKEM/flowkit/core"
    "github.com/gin-gonic/gin"
)

type MyReq struct {
    URL string `json:"url"`
}

type MyResp struct {
    Result string `json:"result"`
}

func main() {
    r := gin.Default()

    serve.HTTP(r, "/api/process", serve.HTTPConfig[MyReq, MyResp]{
        App: serve.Func(func(ctx context.Context, req MyReq) (MyResp, error) {
            rc := core.NewRunContext(ctx, uuid.NewString())
            rc.WithValue("url", req.URL)

            report, err := myPipeline.Run(rc, "start")
            if err != nil {
                return MyResp{}, err
            }
            return MyResp{Result: rc.Values["result"].(string)}, nil
        }),
    })

    r.Run(":8080")
}
```

---

## 场景五：SSE 长任务 + 实时进度

适用于需要推送实时进度或需要用户交互的长时间任务：

```go
import "github.com/KKKKKKKEM/flowkit/builtin/serve"

func main() {
    r := gin.Default()

    serve.SSE(r, "/api/run", serve.SSEConfig[MyReq, MyResp]{
        App: serve.Func(myFunc),
        OnStart: func(sess *serve.SSESession, rc *core.RunContext, req MyReq) {
            rc.WithReporter(mySSEReporter(sess))
        },
    })

    r.Run(":8080")
}
```

**客户端接入示例（JavaScript）：**

```javascript
// 启动任务
const eventSource = new EventSource('/api/run');
let sessionId = '';

eventSource.addEventListener('session', (e) => {
    const data = JSON.parse(e.data);
    sessionId = data.session_id;
});

eventSource.addEventListener('progress', (e) => {
    const data = JSON.parse(e.data);
    console.log('进度:', data);
});

eventSource.addEventListener('interact', async (e) => {
    const data = JSON.parse(e.data);
    // 收到交互请求，展示给用户
    const userAnswer = await showInteractionUI(data.interaction);
    
    // 提交答案
    await fetch('/api/run/answer', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'SESSION-ID': sessionId,
        },
        body: JSON.stringify({
            interaction_id: data.interaction_id,
            result: { answer: userAnswer },
        }),
    });
});

eventSource.addEventListener('done', (e) => {
    const result = JSON.parse(e.data);
    console.log('完成:', result);
    eventSource.close();
});

eventSource.addEventListener('error', (e) => {
    console.error('错误:', JSON.parse(e.data));
    eventSource.close();
});
```

**断线重连（利用 `Last-Event-ID`）：**

```javascript
// 使用 Last-Event-ID header 重连，服务端会重放错过的事件
fetch('/api/run', {
    method: 'POST',
    headers: {
        'SESSION-ID': sessionId,
        'Last-Event-ID': lastEventId, // 上次收到的最后事件序号
    },
});
```

---

## 场景六：网页内容抓取（x/grasp）

### CLI 模式（终端交互 + 进度条）

```go
package main

import (
    "context"

    "github.com/KKKKKKKEM/flowkit/builtin/download"
    "github.com/KKKKKKKEM/flowkit/builtin/extract"
    "github.com/KKKKKKKEM/flowkit/x/grasp"
)

func main() {
    // 组装解析器
    extractor := extract.NewStage("extractor")
    extractor.Mount(&MyExtractor{})

    // 创建 Pipeline
    p := grasp.NewGraspPipeline(
        grasp.WithExtractor(extractor),
        grasp.WithDownloader(download.NewStage("download")),
        grasp.WithPlugin(grasp.CLISelectPlugin{}),  // 终端选择
        grasp.WithProgress(grasp.NewMpbReporter()), // 进度条
    )

    // 定义任务
    task := &grasp.Task{
        URLs: []string{"https://example.com/gallery"},
        Extract: grasp.ExtractConfig{
            MaxRounds:   2,   // 列表页 → 详情页
            Concurrency: 4,
        },
        Download: grasp.DownloadConfig{
            Dest:        "./downloads",
            Concurrency: 8,
        },
        Selector: grasp.SelectFirst(10), // 只下载前 10 个
    }

    report, err := p.Invoke(context.Background(), task)
    if err != nil {
        panic(err)
    }
    
    // report.Downloaded 包含所有已下载文件的路径和大小
}
```

### Web 模式（REST + SSE）

```go
func main() {
    p := grasp.NewGraspPipeline(
        grasp.WithExtractor(extractor),
        grasp.WithDownloader(download.NewStage("download")),
        grasp.WithPlugin(grasp.WebSelectPlugin{}), // SSE 交互
    )

    // 一行启动服务
    p.Serve(":8080")
    
    // 或者挂载到已有 Gin 路由
    // p.GinRegister(r.Group("/grasp"))
}
```

### 实现自定义解析器

```go
import (
    "context"
    "regexp"

    "github.com/KKKKKKKEM/flowkit/builtin/extract"
)

type MyExtractor struct{}

func (e *MyExtractor) Name() string { return "my-site" }

func (e *MyExtractor) Handlers() []*extract.Parser {
    return []*extract.Parser{
        {
            // 匹配列表页
            Pattern:  regexp.MustCompile(`^https://mysite\.com/list`),
            Priority: 10,
            Hint:     "list",
            Parse: func(ctx context.Context, task *extract.Task, opts *extract.Opts) ([]extract.ParseItem, error) {
                // 解析列表页，返回详情页 URL
                return []extract.ParseItem{
                    {URI: "https://mysite.com/item/1", IsDirect: false}, // 继续解析
                    {URI: "https://mysite.com/item/2", IsDirect: false},
                }, nil
            },
        },
        {
            // 匹配详情页
            Pattern:  regexp.MustCompile(`^https://mysite\.com/item/`),
            Priority: 5,
            Hint:     "detail",
            Parse: func(ctx context.Context, task *extract.Task, opts *extract.Opts) ([]extract.ParseItem, error) {
                // 解析详情页，返回直接下载链接
                return []extract.ParseItem{
                    {
                        Name:     "photo.jpg",
                        URI:      "https://cdn.mysite.com/photo.jpg",
                        IsDirect: true, // 直接下载
                        Meta:     map[string]any{"width": 1920, "height": 1080},
                    },
                }, nil
            },
        },
    }
}
```

---

## 中间件使用

### 标准推荐配置

```go
logger := log.New(os.Stdout, "[flowkit] ", log.LstdFlags)
policy := &core.DefaultErrorPolicy{MaxRetries: 3}
metrics := make(map[string]map[string]float64)

lp.Use(
    middleware.RecoveryMiddleware(logger),          // 1. 最外层：捕获 panic
    middleware.LoggingMiddleware(logger),           // 2. 记录日志（含耗时）
    middleware.MetricsMiddleware(metrics),          // 3. 采集指标
    middleware.TimeoutMiddleware(30*time.Second),   // 4. 超时控制
    middleware.RetryMiddleware(policy),             // 5. 最内层：重试
)
```

### 自定义中间件

```go
func AuthMiddleware(token string) core.Middleware {
    return func(next core.StageRunner) core.StageRunner {
        return func(rc *core.RunContext, st core.Stage) core.StageResult {
            if rc.Values["auth_token"] != token {
                return core.StageResult{
                    Status: core.StageFailed,
                    Err:    errors.New("unauthorized"),
                }
            }
            return next(rc, st)
        }
    }
}
```

---

## 最佳实践

### 1. Stage 命名规范

使用描述性名称，避免空格（FSM 模式通过字符串引用）：

```go
// 推荐
func (s *FetchDataStage) Name() string { return "fetch-data" }

// 避免
func (s *S1) Name() string { return "stage 1" }
```

### 2. RunContext 共享状态约定

建议在业务层定义常量键名，避免魔法字符串：

```go
const (
    KeyURL     = "url"
    KeyData    = "raw_data"
    KeyParsed  = "parsed_result"
)

// 写入
return core.StageResult{
    Outputs: map[string]any{KeyData: data},
}

// 读取
data := rc.Values[KeyData].([]byte)
```

### 3. 错误处理策略

- **网络请求失败** → 返回 `StageFailed`，让 `RetryMiddleware` 处理重试
- **参数验证失败** → 返回 `StageFailed`，不应重试
- **panic** → 由 `RecoveryMiddleware` 捕获，无需手动处理
- **跳过逻辑** → 返回 `StageSkipped`（不影响 Pipeline 成功状态）

```go
func (s *MyStage) Run(rc *core.RunContext) core.StageResult {
    data, ok := rc.Values["data"]
    if !ok {
        return core.StageResult{
            Status: core.StageFailed,
            Err:    fmt.Errorf("missing required key: data"),
        }
    }

    result, err := process(data)
    if err != nil {
        return core.StageResult{
            Status: core.StageFailed,
            Err:    fmt.Errorf("process failed: %w", err),
        }
    }

    return core.StageResult{
        Status:  core.StageSuccess,
        Outputs: map[string]any{"result": result},
        Metrics: map[string]float64{"items_processed": float64(len(result))},
    }
}
```

### 4. TraceID 传播

TraceID 自动贯穿整个 Pipeline，可以集成到日志系统：

```go
logger.Printf("[%s] Processing: %s", rc.TraceID, url)
```

### 5. 进度上报

在需要实时进度的 Stage 中，通过 `rc.Reporter()` 上报：

```go
func (s *DownloadStage) Run(rc *core.RunContext) core.StageResult {
    reporter := rc.Reporter()
    if reporter != nil {
        tracker := reporter.Track("my-file.zip", totalSize)
        defer tracker.Done()
        // 定期调用 tracker.Update(downloaded)
    }
    // ...
}
```
