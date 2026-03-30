# 示例：x/grasp

`x/grasp` 是当前仓库里最完整的真实应用实现，也是最推荐的阅读入口。

## 最小启动示例

文件：`examples/grasp/main.go`

```go
func main() {
    p := grasp.NewGraspPipeline()

    if err := p.Launch(); err != nil {
        log.Fatal(err)
    }
}
```

---

## 它做了什么

`x/grasp` 组合了：

- `extract.Stage`
- `download.Stage`
- CLI interaction plugin
- MPB tracker provider

并提供完整的资源解析与下载流程。

## Pipeline 结构

定义在：`x/grasp/pipeline.go`

核心组成：

- `flowkit.App[*Task, *Report]`
- `pipeline.LinearPipeline`
- `extract.Stage`
- `download.Stage`

它既可以：

- `CLI()` 启动
- `Serve()` 启动
- `Launch()` 自动分发启动

---

## `grasp.Task`

定义在：`x/grasp/task.go`

主要配置包括：

- `URLs`
- `Proxy`
- `Timeout`
- `Retry`
- `Headers`
- `Extract.MaxRounds`
- `Extract.WorkerConcurrency`
- `Download.TaskConcurrency`
- `Download.SegmentConcurrency`
- `Download.BestEffort`
- `Download.ChunkSize`

---

## `grasp` CLI 参数

定义在：`x/grasp/cli.go`

当前支持：

- `-url`
- `-proxy`
- `-timeout`
- `-retry`
- `-header`
- `-rounds`
- `-extract-concurrency`
- `-dest`
- `-overwrite`
- `-download-task-concurrency`
- `-best-effort`
- `-download-segment-concurrency`
- `-chunk-size`

---

## 为什么它重要

如果你想理解：

- 如何创建一个真实的 Flowkit 应用
- 如何组织 stage
- 如何把 Launch / CLI / Serve 接起来
- 如何处理交互与进度追踪

那优先读 `x/grasp/`。
