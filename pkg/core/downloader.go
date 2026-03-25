package core

import (
	"context"
	"io"
	"time"
)

type DownloadTask struct {
	URL         string
	Dest        string
	Headers     map[string]string
	Concurrency int

	// Proxy 指定下载使用的代理地址，支持 http://、https://、socks5:// 格式。
	// 优先级高于 Meta["proxy"]。空字符串表示不使用代理。
	// 特殊值 "env" 表示自动读取系统环境变量（HTTP_PROXY / HTTPS_PROXY / NO_PROXY）。
	Proxy string

	// Timeout 为单次 HTTP 请求（含 HEAD probe）的超时时间。0 表示不限制。
	Timeout time.Duration

	// Retry 为下载失败时的最大重试次数（不含首次），0 表示不重试。
	Retry int

	// RetryInterval 为相邻两次重试之间的等待时间，默认 1s。
	RetryInterval time.Duration

	OnProgress ProgressFunc
	OnComplete CompleteFunc
	OnError    ErrorFunc
	Meta       map[string]any
}

// CompleteFunc 在下载成功后调用，result 包含实际文件路径和写入字节数。
type CompleteFunc func(result *DownloadResult)

// ProgressFunc 报告下载进度；total 为 -1 表示总大小未知。
type ProgressFunc func(downloaded, total int64)

// ErrorFunc 在下载失败时调用，err 为实际错误原因。
type ErrorFunc func(err error)

type DownloadResult struct {
	FilePath     string
	BytesWritten int64
}

type Downloader interface {
	// CanHandle 根据 task 的任意字段综合判断，不应发起网络请求。
	CanHandle(task *DownloadTask) bool
	Download(ctx context.Context, task *DownloadTask) (*DownloadResult, error)
	Name() string
}

// StreamDownloader 是可选扩展，适用于不落盘直接消费字节流的场景。
type StreamDownloader interface {
	Downloader
	Stream(ctx context.Context, task *DownloadTask) (io.ReadCloser, error)
}
