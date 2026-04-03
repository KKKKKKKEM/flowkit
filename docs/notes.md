# 注意事项

---

## 1. 推荐使用 `Launch` 而不是手动分支

在入口 `main.go` 里直接用 `app.Launch()`，不要在 Builder 或业务逻辑里再做模式判断。

---

## 2. go.mod 版本问题

`go.mod` 当前写的是 `go 1.25.0`（无效版本号），会导致 `go list`、`gopls`、部分 IDE 诊断失败。

本地编译请用 Go 1.22+ 的实际工具链：

```bash
~/sdk/go1.23.5/bin/go build ./...
```

后续建议将 `go.mod` 改为实际支持的最低版本（如 `go 1.22.0`）。

---

## 3. AutoFlags 的类型限制

`cli.ParseFlags` / `ParseFlagsPtr` 支持以下类型：

- 基础类型：`string`、`bool`、`int`、`int64`、`float64`、`time.Duration`
- 复合类型：`[]string`、`map[string]string/int/int64/float64/bool`
- 嵌套结构体（`time.Duration` 除外，不作为嵌套处理）

不在上述范围内的类型（如自定义类型、`[]int` 等）会在运行时返回 error。对这类字段，加 `cli:"-"` 跳过，或改用手动 `WithCLIBuilder`。

---

## 4. `required` 字段与默认值的关系

如果字段同时声明了 `required` 和 `default=...`，default 值会在 `ParseFlags` 阶段被 set，`required` 检查基于 zero 值判断，因此**有 default 的字段不会触发 required 错误**。这是预期行为。

---

## 5. SSE Session 过期行为

`SessionStore.GetOrCreate` 的策略：

- 不带 `SESSION-ID` → 新建 Session
- 带已存在的 `SESSION-ID` → 返回已有 Session（断线重连）
- 带不存在的 `SESSION-ID` → 返回 `404` error，前端应不带 ID 重新发起（不会静默新建）

这个设计是有意为之：防止前端意外用过期 ID 创建出"幽灵 Session"。

---

## 6. Track 事件去重

SSE `track` 事件按 `tag` 去重，同一 tag 的历史事件**不会在断线重连时全量重放**，只保留最新一条。这样设计是为了避免大量进度事件在重连时堵塞连接。

`interact`、`done`、`error` 是关键事件，全量保留，断线后会补发。

---

## 7. x/grasp 是最完整的参考实现

如果想理解 Flowkit 的完整用法，优先阅读：

- `x/grasp/pipeline.go` — 组装方式（WithCLIAutoFlags 接入）
- `x/grasp/task.go` — 请求结构设计（cli tag + 嵌套前缀）
- `x/grasp/track.go` — mpb 进度条接入
- `x/grasp/interact.go` — 终端交互接入

---

## 8. 与现有代码同步

如果继续演进，优先保持以下内容与代码同步：

| 代码位置 | 文档 |
|----------|------|
| `app.go` Launch/CLI/Serve 语义 | [启动方式](./launch-and-runtime.md) |
| `cli/flags.go` tag 格式与支持类型 | [启动方式](./launch-and-runtime.md)（AutoFlags 小节） |
| `server/sse.go` 会话协议 | [HTTP/SSE 模型](./server-and-sse.md) |
| `server/sse/types.go` 事件类型 | [HTTP/SSE 模型](./server-and-sse.md) |
| `x/grasp/task.go` cli tag 设计 | [示例：x/grasp](./grasp.md) |
| `stages/download` 失败策略 | [内置 Stages](./stages.md) |
