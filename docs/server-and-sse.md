# HTTP / SSE 交互模型

Flowkit 的服务端不仅能返回 JSON，也能以 SSE 会话的方式持续推送运行事件。

## 普通 HTTP adapter

定义在：`server/http.go`

能力：

- 从 HTTP 请求构建 `Req`
- 创建 `core.Context`
- 调用 `App.Invoke`
- 直接返回 JSON `Resp`

适合：

- 无交互
- 无实时进度推送
- 简单 request/response 场景

---

## SSE adapter

定义在：`server/sse.go`

能力：

- 创建会话
- 异步执行 `App.Invoke`
- 推送 tracker / interaction / done / error 事件
- 支持交互结果通过 HTTP 回答再回注运行时

### 路由

#### `POST {path}/stream`

作用：

- 创建或恢复 SSE session
- 启动应用执行
- 持续推送事件流

#### `POST {path}/answer`

作用：

- 将用户对交互请求的回答回注到运行中的 session

请求体格式：

```json
{
  "interaction_id": "...",
  "result": { ... }
}
```

### 关键 header

- `SESSION-ID`
- `LAST-EVENT-ID`

用于：

- 会话恢复
- 断线续流

---

## 为什么 SSE 很重要

因为这使得同一套业务逻辑中的交互行为可以跨 transport 存活：

- 在 CLI 中，交互可以表现为终端阻塞输入
- 在 Web 中，交互可以表现为 SSE 推送问题，前端再回调 `/answer`

这也是 Flowkit 支持“同一套业务逻辑，终端交互与 Web 交互共存”的关键。
