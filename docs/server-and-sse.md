# HTTP / SSE 交互模型

Flowkit 的服务端基于 SSE 会话，不只是简单的 JSON request/response —— 它支持流式事件推送和双向交互回传。

---

## 路由

`server.SSE(router, path, cfg)` 注册两条路由：

| 路由 | 方法 | 说明 |
|------|------|------|
| `{path}/stream` | POST | 创建/恢复 SSE 会话，持续推送事件流 |
| `{path}/answer` | POST | 将用户交互回答注入正在运行的会话 |

默认 path 为 `/app`，即 `/app/stream` 和 `/app/answer`。

---

## 连接流程

```
Client                              Server
  │                                    │
  │  POST /app/stream                  │
  │  (Header: SESSION-ID: "")          │
  │ ─────────────────────────────────> │  创建新 Session
  │                                    │  异步启动 App.Invoke
  │ <───────────────── event: session  │  立即返回 SESSION-ID
  │ <──────────────── event: track ... │  进度事件（持续推送）
  │ <─────────────── event: interact   │  需要用户选择
  │                                    │
  │  POST /app/answer                  │
  │  (Header: SESSION-ID: "xxx")       │
  │  Body: {interaction_id, result}    │
  │ ─────────────────────────────────> │  解除 Invoke 阻塞
  │                                    │
  │ <──────────────── event: track ... │  下载进度
  │ <───────────────────── event: done │  最终结果（Resp JSON）
```

---

## 5 种事件类型

SSE 事件格式：

```
id: <seq>
event: <type>
data: <json>
```

| 事件类型 | 触发时机 | data 内容 |
|----------|----------|-----------|
| `session` | 连接建立时，始终第一条 | `{"SESSION-ID": "uuid"}` |
| `track` | TrackerProvider 调用时 | `{"tag": "...", "current": N, "total": N}` |
| `interact` | InteractionPlugin 触发时 | `{"interaction_id": "...", "interaction": {...}}` |
| `done` | `App.Invoke` 正常返回 | Resp 的 JSON 序列化 |
| `error` | `App.Invoke` 返回 error | `{"message": "..."}` |

`track` 事件按 `tag` 去重——同一 tag 只保留最新一条（断线重连时不会回放大量过期进度事件）。

`done` / `error` 事件发出后 Session 立即从 Store 中删除。

---

## 断线重连

重连时携带上次收到的最大事件序号：

```http
POST /app/stream
SESSION-ID: <existing-session-id>
LAST-EVENT-ID: <last-seq>
```

Server 会重放 `seq > LAST-EVENT-ID` 的未推送事件（`track` 只取每个 tag 最新，`interact`/`done`/`error` 全量保留）。

若 Session 已过期（默认 TTL 30 分钟），Server 返回 `404`，前端应不带 SESSION-ID 重新发起。

---

## 交互回答

当业务逻辑调用 `InteractionPlugin.Interact()`，SSE 端会：

1. 推送 `interact` 事件，携带 `interaction_id` 和 `Interaction` 结构
2. **阻塞** Invoke goroutine 等待回答

前端收到 `interact` 事件后，展示选择 UI，用户确认后：

```http
POST /app/answer
SESSION-ID: <session-id>
Content-Type: application/json

{
  "interaction_id": "xxx",
  "result": { "answer": [0, 2, 5] }
}
```

---

## 自定义 SessionStore

```go
store := sse.NewSSESessionStore(
    30*time.Minute,     // TTL（多久清理已完成 session）
    7*24*time.Hour,     // 最大存活时间
)

app.Serve(":8080",
    flowkit.WithStore[*Req, *Resp](store),
)
```

默认值：TTL = 30 分钟，最大存活 = 7 天。

---

## 禁用内置插件

SSE adapter 默认自动注入 `SSETrackerProvider` 和 `SSEInteractionPlugin`。如果你想自己控制，或不需要这些功能：

```go
app.Serve(":8080",
    flowkit.DisableTrackerProvider[*Req, *Resp](),
    flowkit.DisableInteractionPlugin[*Req, *Resp](),
)
```

---

## 为什么用 SSE 而不是 WebSocket

- SSE 是单向推送，连接模型比 WebSocket 简单
- 断线重连由 HTTP 层天然支持（`Last-Event-ID`）
- 交互回答走独立的 `POST /answer`，不需要双向 socket
- 与任意反向代理（nginx、caddy）天然兼容（只需关闭 buffering）
