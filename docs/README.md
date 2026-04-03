# Flowkit 文档

## 文档导航

| 文档 | 内容摘要 |
|------|----------|
| [架构概览](./architecture.md) | 层次模型（App→Pipeline→Stage）、数据流、各模块职责 |
| [核心概念](./core-concepts.md) | `Context`、`SharedState`、`Stage`、`TypedStage`、`Middleware` 接口详解 |
| [Pipeline 模式](./pipelines.md) | `LinearPipeline` 顺序执行、`FSMPipeline` 状态跳转、Middleware 挂载 |
| [内置 Stages](./stages.md) | `cond`、`fan`、`extract`、`download` 用法与接口契约 |
| [启动方式](./launch-and-runtime.md) | `Launch`/`CLI`/`Serve` 全量配置项、命令行参数约定 |
| [HTTP/SSE 模型](./server-and-sse.md) | SSE 协议、5 种事件类型、交互流程、断线重连 |
| [示例：x/grasp](./grasp.md) | grasp 流水线完整解析：extract→select→transform→download |
| [扩展指南](./extending-flowkit.md) | 自定义 Parser、Downloader、TrackerProvider、InteractionPlugin |
| [注意事项](./notes.md) | 已知边界、版本约束、常见误用 |

## 阅读路径

**初次了解** → [架构概览](./architecture.md) → [核心概念](./core-concepts.md)

**集成 HTTP/SSE** → [启动方式](./launch-and-runtime.md) → [HTTP/SSE 模型](./server-and-sse.md)

**扩展 extract/download** → [内置 Stages](./stages.md) → [扩展指南](./extending-flowkit.md)

**参考完整实现** → [示例：x/grasp](./grasp.md)
