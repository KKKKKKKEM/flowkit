# 启动方式

## Launch

`App.Launch(...)` 是推荐的统一入口。

当前默认支持两种模式：

- `run ...` → CLI 模式
- `serve --addr=:8080` → HTTP 模式

并保留一个便捷行为：

- 裸参数默认等价于 `run ...`

例如：

```bash
app run -url https://example.com
app serve --addr=:8080
app -url https://example.com
```

## Launch 的内部模型

`Launch` 内部维护显式双配置：

- 一份 CLI config
- 一份 Serve config

Launch 自己只负责：

- 模式解析
- 启动分发

它不会把 server 逻辑藏进 CLI adapter 里。

## 配置方式

### CLI 侧配置

通过：

- `WithLaunchCLIOptions(...)`

复用已有的 `CLIOption`：

- `WithCLIBuilder(...)`
- `WithCLIArgs(...)`
- `WithTrackerProvider(...)`
- `WithInteractionPlugin(...)`
- `WithOnResult(...)`
- `WithOnError(...)`

### Server 侧配置

通过：

- `WithLaunchServeOptions(...)`

复用已有的 `ServeOption`：

- `WithEngine(...)`
- `WithPath(...)`
- `WithStore(...)`
- `WithServeBuilder(...)`
- `WithOnStart(...)`
- `DisableTrackerProvider()`
- `DisableInteractionPlugin()`

### Launch 自己的配置

- `WithModeResolver(...)`
- `WithDefaultHTTPAddr(...)`

---

## CLI

`App.CLI(...)` 是纯 CLI transport adapter。

负责：

- 从 args 构建请求对象
- 创建 `core.Context`
- 注入 tracker / interaction plugin
- 调用 `App.Invoke`
- 输出结果到 stdout

示例：

```go
app.CLI(
    flowkit.WithCLIBuilder(buildCLI),
)
```

---

## Serve

`App.Serve(...)` 是服务端入口。

当前默认基于 `server.SSE(...)`，也就是说它默认提供的是：

- 会话
- 流式事件
- 交互回传

而不是简单的一次性 REST 调用。

示例：

```go
app.Serve(":8080",
    flowkit.WithPath("/app"),
)
```

---

## 最小启动示例

```go
app := flowkit.NewApp(func(ctx *core.Context, req *Req) (*Resp, error) {
    return &Resp{Message: "hello " + req.Name}, nil
})

if err := app.Launch(
    flowkit.WithLaunchCLIOptions(
        flowkit.WithCLIBuilder(buildCLI),
    ),
    flowkit.WithLaunchServeOptions(
        flowkit.WithPath("/app"),
    ),
); err != nil {
    log.Fatal(err)
}
```
