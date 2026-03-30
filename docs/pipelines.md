# Pipeline 模式

## Linear Pipeline

定义在：`pipeline/linear.go`

特点：

- 按注册顺序执行
- 从 `entry` 开始往后执行
- 任一 stage 失败则中断
- 每个 stage 的 `Outputs` 自动 merge 到共享状态

适合：

- 顺序处理流程
- 提取 → 选择 → 转换 → 下载 这类直线型业务

---

## FSM Pipeline

定义在：`pipeline/fsm.go`

特点：

- 根据 `StageResult.Next` 决定下一跳
- 支持状态机式编排
- 内置 `maxVisits` 防环保护

适合：

- 条件分支
- 多状态流转
- 可恢复/可重复进入的流程

---

## 如何选择

### 选择 Linear Pipeline

如果你的流程是固定顺序：

- Extract
- Select
- Transform
- Download

优先用 Linear。

### 选择 FSM Pipeline

如果你的流程需要根据运行结果决定下一跳：

- 条件跳转
- 失败分流
- 有状态循环

用 FSM。
