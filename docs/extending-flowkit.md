# 扩展指南

## 1. 新建一个 App

如果你只是想把一段业务逻辑同时暴露为 CLI 和 HTTP：

- 用 `flowkit.NewApp(...)`
- 用 `Launch()` 统一启动

---

## 2. 新建一个 TypedStage

如果你有清晰的输入输出：

- 实现 `TypedStage[In, Out]`
- 用 `core.NewTypedStage(...)` 适配为普通 `Stage`

---

## 3. 新建一个 Pipeline

如果你需要组织多个阶段：

- 简单顺序流 → `pipeline.NewLinearPipeline()`
- 状态驱动流 → `pipeline.NewFSMPipeline()`

---

## 4. 新建 transport-specific 插件

如果你想自定义：

- 进度推送方式
- 用户交互方式

可以实现：

- `core.TrackerProvider`
- `core.InteractionPlugin`

CLI 和 SSE 就是当前两套现成实现。

---

## 5. 扩展内置 stages

建议方式：

- 控制流类能力优先做成普通 `Stage`
- 数据处理类能力优先做成 `TypedStage`
- 把输入输出边界尽量显式化
- 把默认值解析与 fallback 机制单独抽出，而不是散落在业务逻辑中
