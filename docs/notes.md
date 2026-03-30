# 注意事项

## 1. 启动方式建议

当前推荐直接使用：

- `App.Launch()`

而不是在 CLI 内部继续做模式切换。

## 2. `x/grasp` 是当前最重要的参考实现

如果你想看：

- 如何组装 pipeline
- 如何配置 Launch
- 如何接入 tracker / interaction
- 如何做真实下载流程

优先读：`x/grasp/`

## 3. 当前仓库存在 Go toolchain 配置问题

`go.mod` 当前写的是：

```go
go 1.25.0
```

如果你的本地工具链版本较低，可能会导致：

- `go list`
- `gopls`
- 部分构建/诊断

直接失败。

在当前开发环境里，这一点会影响完整包加载验证。

## 4. 文档同步建议

如果后续继续演进，优先保持以下内容与代码同步：

- `app.go` 中的 Launch 语义
- `x/grasp/cli.go` 中的 CLI 参数
- `server/sse.go` 中的会话接口约定
- `stages/download` 中的失败策略与并发语义
