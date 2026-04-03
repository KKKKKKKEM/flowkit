# 扩展指南

---

## 1. 最简 App

将业务逻辑同时暴露为 CLI 和 HTTP：

```go
type Req struct {
    Query string `cli:"query,required,search query"`
}
type Resp struct {
    Results []string `json:"results"`
}

app := flowkit.NewApp(func(ctx *core.Context, req *Req) (*Resp, error) {
    // 业务逻辑
    return &Resp{Results: search(req.Query)}, nil
})

app.Launch(
    flowkit.WithLaunchCLIOptions(
        flowkit.WithCLIAutoFlags[*Req, *Resp](),
    ),
)
```

---

## 2. 自定义 Stage

### 普通 Stage

适合控制流、无明确 In/Out 的阶段：

```go
type MyStage struct{}

func (s *MyStage) Name() string { return "my-stage" }

func (s *MyStage) Run(rc *core.Context) core.StageResult {
    data, ok := core.GetState[*MyData](rc, "input")
    if !ok {
        return core.StageResult{Status: core.StageFailed, Err: fmt.Errorf("input missing")}
    }

    result := process(data)
    core.SetState(rc, "output", result)
    return core.StageResult{Status: core.StageSuccess}
}
```

### TypedStage

适合数据处理类，输入输出类型明确：

```go
type TransformStage struct{}

func (s *TransformStage) Name() string { return "transform" }

func (s *TransformStage) Exec(rc *core.Context, in *Input) (core.TypedResult[*Output], error) {
    out := &Output{Value: transform(in.Value)}
    return core.TypedResult[*Output]{Output: out}, nil
}

// 适配为普通 Stage
stage := core.NewTypedStage[*Input, *Output](&TransformStage{}, "input-key", "output-key")
```

---

## 3. 组装 Pipeline

### LinearPipeline — 顺序执行

```go
p := pipeline.NewLinearPipeline(
    pipeline.WithMiddleware(loggingMiddleware),
)
p.Register(&StageA{})
p.Register(&StageB{})
p.Register(&StageC{})

// 用 App 包装
app := flowkit.NewApp(func(ctx *core.Context, req *Req) (*Resp, error) {
    core.SetState(ctx, "req", req)
    report, err := p.Run(ctx, "run")
    if err != nil {
        return nil, err
    }
    return core.GetState[*Resp](ctx, "resp")
})
```

### FSMPipeline — 状态跳转

```go
p := pipeline.NewFSMPipeline()
p.Register(&InitStage{})        // 返回 Next: "process"
p.Register(&ProcessStage{})     // 返回 Next: "done" 或 "retry"
p.Register(&RetryStage{})       // 返回 Next: "process"
p.Register(&DoneStage{})        // 返回 Next: ""（终止）
```

---

## 4. 自定义 TrackerProvider

在 CLI 或自定义 UI 中显示进度：

```go
type MyTrackerProvider struct{}

func (p *MyTrackerProvider) Track(name string, meta map[string]any) core.Tracker {
    return &MyTracker{name: name}
}

func (p *MyTrackerProvider) Wait() {
    // 等待所有 tracker 完成（如 mpb.Progress.Wait()）
}

type MyTracker struct{ name string }

func (t *MyTracker) Update(meta map[string]any) {
    current, _ := meta["current"].(int64)
    total, _ := meta["total"].(int64)
    fmt.Printf("\r%s: %d/%d", t.name, current, total)
}

func (t *MyTracker) Flush() {}

func (t *MyTracker) Done() {
    fmt.Printf("\n%s: done\n", t.name)
}
```

注入：

```go
app.CLI(
    flowkit.WithTrackerProvider[*Req, *Resp](&MyTrackerProvider{}),
)
```

---

## 5. 自定义 InteractionPlugin

在 CLI 中阻塞等待用户输入：

```go
type MyInteractionPlugin struct{}

func (p *MyInteractionPlugin) Interact(rc *core.Context, i core.Interaction) (core.InteractionResult, error) {
    switch i.Type {
    case core.InteractionTypeSelect:
        items := i.Payload.([]extract.Item)
        // 展示列表，读取用户输入
        indices := promptSelect(items)
        return core.InteractionResult{Answer: indices}, nil
    }
    return core.InteractionResult{}, fmt.Errorf("unsupported interaction type: %s", i.Type)
}

func (p *MyInteractionPlugin) FormatResult(rc *core.Context, i core.Interaction, r core.InteractionResult) (core.InteractionResult, error) {
    return r, nil
}
```

---

## 6. 自定义 extract.Parser

为新站点添加解析支持：

```go
type MyParser struct{}

func (p *MyParser) Name() string { return "my-site" }

func (p *MyParser) Match(url string) bool {
    return strings.Contains(url, "mysite.com")
}

func (p *MyParser) Parse(ctx *core.Context, task *extract.Task) ([]extract.Item, error) {
    // 发起请求，解析响应，返回资源列表
    return []extract.Item{
        {URI: "https://cdn.mysite.com/file.mp4", Name: "file.mp4", IsDirect: true},
    }, nil
}
```

注册：

```go
extractor := extract.NewStage("extractor")
extractor.Mount(&MyParser{})
```

---

## 7. 自定义 download.Downloader

支持非 HTTP 协议或特殊认证：

```go
type MyDownloader struct{}

func (d *MyDownloader) Name() string { return "my-proto" }

func (d *MyDownloader) Match(uri string) bool {
    return strings.HasPrefix(uri, "myproto://")
}

func (d *MyDownloader) Download(ctx *core.Context, task *download.Task) (*download.Result, error) {
    // 实现下载逻辑
    return &download.Result{Path: task.Opts.Dest + "/" + task.Name}, nil
}
```

注册：

```go
downloader := download.NewStage("download",
    download.WithDownloaders(
        dlhttp.NewHTTPDownloader(),
        &MyDownloader{},
    ),
)
```

---

## 扩展原则

- **控制流** → 普通 `Stage`（Run 返回 `StageResult`）
- **数据处理** → `TypedStage[In, Out]`（Exec 返回强类型）
- **进度** → 实现 `TrackerProvider` + `Tracker`
- **交互** → 实现 `InteractionPlugin`
- **新协议/站点** → 实现 `extract.Parser` 或 `download.Downloader`
- 把默认值解析与 fallback 机制显式化，不要散落在业务逻辑中
