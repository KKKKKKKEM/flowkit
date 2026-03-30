# 内置 Stages

## `stages/cond`

条件跳转 stage，适用于 FSM。

- 顺序评估分支
- 命中第一个条件即跳转
- 未命中走 fallback

适合：

- 路由到不同分支
- 表达状态机中的条件决策

---

## `stages/fan`

并发子阶段执行器。

支持：

- `wait all + fail fast`
- `wait any`
- `best effort`
- 输出冲突合并策略

适合：

- 扇出执行多个子 stage
- 并行探测 / 并行抓取 / 并行提取

---

## `stages/extract`

typed data stage。

职责：

- 根据 URL / forced parser 选择 parser
- 执行内容提取
- 输出 `[]Item`

当前支持：

- 默认值解析（`ResolveOpts`）
- parser 挂载（`Mount`）
- typed stage adapter

适合：

- 网页解析
- 数据抽取
- 多 parser 匹配场景

---

## `stages/download`

typed data stage。

职责：

- 接收下载任务列表
- 根据 URI 派发 downloader
- 执行批量下载

当前语义包括：

- batch 级失败策略
  - `BatchFailFast`
  - `BatchBestEffort`
- batch 级最大并发
- segment 级并发下载
- 结构化 `BatchError`
- 按代理分桶复用 HTTP transport
- 统一 defaults / resolved opts 机制

适合：

- 文件下载
- 分片下载
- 多任务批量下载

---

## 当前设计特点

当前内置 stage 形成了两类能力：

### 控制流阶段

- `cond`
- `fan`

### typed 数据处理阶段

- `extract`
- `download`

这是当前仓库里已经成型的双轨模型。
