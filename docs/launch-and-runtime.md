# 启动方式

Flowkit 提供三种入口，均通过 `App[Req, Resp]` 暴露。

## 快速对比

| 方法 | 场景 |
|------|------|
| `app.Launch(...)` | **推荐**。自动按 args 分发 CLI / HTTP |
| `app.CLI(...)` | 只需命令行工具 |
| `app.Serve(addr, ...)` | 只需 HTTP/SSE 服务端 |

---

## Launch — 统一入口

```bash
app run -url https://example.com    # CLI 模式
app -url https://example.com        # 等价于 run（裸参数）
app serve --addr=:9000              # HTTP/SSE 模式
app help                            # 打印用法 + flag 列表
```

内部维护两份独立配置（CLI + Serve），只负责模式解析和分发，不做业务耦合。

默认支持以下子命令：

| 子命令 | 说明 |
|--------|------|
| `run [flags]` | CLI 模式执行（等价于裸传 flags） |
| `serve [--addr=:8080]` | 启动 HTTP/SSE 服务 |
| `help` | 打印用法说明 + flag 列表（若使用 AutoFlags 则自动生成） |

### 完整配置

```go
app.Launch(
    // CLI 侧
    flowkit.WithLaunchCLIOptions(
        flowkit.WithCLIBuilder[*Req, *Resp](buildCLI),
        flowkit.WithTrackerProvider[*Req, *Resp](myTracker),
        flowkit.WithInteractionPlugin[*Req, *Resp](myPlugin),
        flowkit.WithOnResult[*Req, *Resp](func(r *Resp) { ... }),
        flowkit.WithOnError[*Req, *Resp](func(err error) { ... }),
    ),
    // Serve 侧
    flowkit.WithLaunchServeOptions(
        flowkit.WithPath[*Req, *Resp]("/api"),
        flowkit.WithEngine[*Req, *Resp](myGin),
        flowkit.WithStore[*Req, *Resp](customStore),
        flowkit.WithOnStart[*Req, *Resp](func(sess *sse.Session, rc *core.Context, req *Req) { ... }),
        flowkit.DisableTrackerProvider[*Req, *Resp](),
        flowkit.DisableInteractionPlugin[*Req, *Resp](),
    ),
    // Launch 自身
    flowkit.WithDefaultHTTPAddr[*Req, *Resp](":9000"),
    flowkit.WithModeResolver[*Req, *Resp](myResolver),
)
```

### 自定义模式解析器

```go
flowkit.WithModeResolver[*Req, *Resp](func(args []string) (flowkit.LaunchPlan, error) {
    if len(args) > 0 && args[0] == "web" {
        return flowkit.LaunchPlan{Mode: flowkit.LaunchModeHTTP, Addr: ":8080"}, nil
    }
    return flowkit.LaunchPlan{Mode: flowkit.LaunchModeCLI, Args: args}, nil
})
```

---

## CLI — 纯命令行

负责：args → `Req` → `App.Invoke` → 结果输出到 stdout（JSON）。

### 手动 Builder

```go
app.CLI(
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
    }),
)
```

### AutoFlags — struct tag 自动解析

无需手写 `flag.FlagSet`，直接用 struct tag 声明 flag：

```go
type Req struct {
    URL     string        `cli:"url,required,usage=target URL"`
    Timeout time.Duration `cli:"timeout,default=30s,usage=request timeout"`
    Dest    string        `cli:"dest,default=./out,usage=output directory"`
    Verbose bool          `cli:"verbose"`
    // 忽略该字段
    Internal string       `cli:"-"`
}

app.CLI(
    flowkit.WithCLIAutoFlags[*Req, *Resp](),
)
```

Flag 名称解析优先级：**cli tag > json tag > 小写字段名**。  
若字段无 `cli` tag 但有 `json` tag，自动用 json 名作为 flag 名（忽略 `omitempty`；`json:"-"` 视为跳过）。

Tag 格式（`cli:"..."`），`usage=` 必须最后，其后内容可含逗号：

| 写法 | 含义 |
|------|------|
| `cli:"name"` | flag 名称 |
| `cli:"name,required"` | 必填，零值时报错 |
| `cli:"name,default=30s"` | 默认值（Duration 用 Go 时间格式） |
| `cli:"name,required,default=./out,usage=output dir"` | 组合，顺序无关（usage= 最后） |
| `cli:"-"` | 跳过该字段 |

支持类型：`string`、`bool`、`int`、`int64`、`float64`、`time.Duration`、`[]string`、`map[string]string/int/int64/float64/bool`。

- `[]string`：flag 可重复传，`-flag a -flag b` → `[]string{"a","b"}`
- `map`：flag 可重复传，`-flag k=v -flag k2=v2`；value 按目标类型解析

嵌套结构体自动加前缀，父字段 tag 名 + `.` 拼接：

```go
type Req struct {
    Download DownloadConfig `cli:"download"`
}
type DownloadConfig struct {
    Dest      string `cli:"dest,default=./out,usage=output directory"`
    Overwrite bool   `cli:"overwrite"`
}
// 对应 flag：-download.dest  -download.overwrite
```

直接复用 json tag（HTTP/CLI 同一个 Req 结构体）：

```go
type Req struct {
    URL     string        `json:"url"`      // CLI flag 名自动为 -url
    Timeout time.Duration `json:"timeout"`  // CLI flag 名自动为 -timeout
    Dest    string        `json:"dest" cli:"dest,default=./out,usage=output dir"` // cli tag 优先
}
```

### 帮助输出

使用 AutoFlags 时，`-h` / `--help` flag 以及 `help` 子命令均会打印完整 flag 列表后正常退出（exit 0）：

```bash
./app -h
./app --help
./app help
./app run -h
```

---

## Serve — HTTP/SSE 服务端

```go
app.Serve(":8080",
    flowkit.WithPath[*Req, *Resp]("/app"),
    flowkit.WithServeBuilder[*Req, *Resp](func(c *gin.Context) (*Req, error) {
        var req Req
        return &req, c.ShouldBindJSON(&req)
    }),
)
```

默认基于 SSE 会话（`server.SSE`），支持流式事件推送和交互回传。详见 [HTTP/SSE 模型](./server-and-sse.md)。

---

## CLIOption 全览

| Option | 说明 |
|--------|------|
| `WithCLIBuilder(fn)` | 手动 Builder，优先级高于 AutoFlags |
| `WithCLIAutoFlags()` | struct tag 自动解析 flag |
| `WithCLIArgs(args)` | 覆盖默认 `os.Args[1:]` |
| `WithTrackerProvider(tp)` | 注入进度追踪器 |
| `WithInteractionPlugin(ip)` | 注入交互插件 |
| `WithOnResult(fn)` | 自定义结果处理（默认 JSON 输出到 stdout） |
| `WithOnError(fn)` | 自定义错误处理（默认 stderr + `os.Exit(1)`） |

## ServeOption 全览

| Option | 说明 |
|--------|------|
| `WithEngine(e)` | 复用已有 `*gin.Engine` |
| `WithPath(p)` | SSE 路由前缀（默认 `/app`） |
| `WithStore(s)` | 自定义 `SessionStore` |
| `WithServeBuilder(fn)` | 从 `*gin.Context` 构建 `Req` |
| `WithOnStart(fn)` | 会话创建后回调 |
| `DisableTrackerProvider()` | 禁用内置 SSE TrackerProvider |
| `DisableInteractionPlugin()` | 禁用内置 SSE InteractionPlugin |
