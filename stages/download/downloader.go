package download

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// Segment / Meta — 断点续传元数据
// ---------------------------------------------------------------------------

type Segment struct {
	Idx     int   `json:"idx"`
	Start   int64 `json:"start"`
	End     int64 `json:"end"`     // inclusive; -1 means stream to EOF
	Written int64 `json:"written"` // bytes successfully written; resume from Start+Written
	Done    bool  `json:"done"`
}

type Meta struct {
	TotalSize int64     `json:"total_size"`
	ChunkSize int64     `json:"chunk_size"`
	Segments  []Segment `json:"segments"`
}

// ---------------------------------------------------------------------------
// Callbacks
// ---------------------------------------------------------------------------

// CompleteFunc 在下载成功后调用，result 包含实际文件路径和写入字节数。
type CompleteFunc func(result *Result)

// ProgressFunc 报告下载进度；total 为 -1 表示总大小未知。
type ProgressFunc func(downloaded, total int64)

// ErrorFunc 在下载失败时调用，err 为实际错误原因。
type ErrorFunc func(err error)

// ---------------------------------------------------------------------------
// Result
// ---------------------------------------------------------------------------

type Result struct {
	Path string `json:"path,omitempty"`
	Size int64  `json:"size,omitempty"`
}

// ---------------------------------------------------------------------------
// Opts — 协议无关的通用下载选项
// ---------------------------------------------------------------------------

type Opts struct {
	Dest string `json:"dest,omitempty"`

	// Proxy 指定代理地址，支持 http://、https://、socks5:// 格式。
	// 特殊值 "env" 表示自动读取系统环境变量（HTTP_PROXY / HTTPS_PROXY / NO_PROXY）。
	Proxy string `json:"proxy,omitempty"`

	// Timeout 为单次请求的超时时间。0 表示不限制。
	Timeout time.Duration `json:"timeout,omitempty"`
	// Retry 为下载失败时的最大重试次数（不含首次），0 表示不重试。
	Retry int `json:"retry,omitempty"`
	// RetryInterval 为相邻两次重试之间的等待时间，默认 1s。
	RetryInterval time.Duration `json:"retry_interval,omitempty"`

	// Overwrite 控制目标文件已存在时的行为，默认 false（跳过）。
	Overwrite bool `json:"overwrite,omitempty"`

	// Concurrency 下载并发数，默认 1（单线程）。大于 1 时启用分块下载（需协议支持）。
	Concurrency int `json:"concurrency,omitempty"`
	// ChunkSize 为分块下载时每个分片的字节数，0 表示使用默认值（1MB）。
	ChunkSize int64 `json:"chunk_size,omitempty"`

	SavePath string `json:"save_path,omitempty"` // 内部使用的实际保存路径，由 Downloader 填充
}

func (o *Opts) Interval() time.Duration {
	if o.RetryInterval > 0 {
		return o.RetryInterval
	}
	return time.Second
}

// ---------------------------------------------------------------------------
// Task — 协议无关的下载任务
// ---------------------------------------------------------------------------

type Task struct {
	URI  string // 统一资源标识符，由各 Downloader 根据 scheme 自行解析
	Opts *Opts

	// 进度回调, 下载进度
	OnProgress ProgressFunc
	// 完成回调, 下载成功后调用
	OnComplete CompleteFunc
	// 错误回调, 下载失败时调用
	OnError ErrorFunc

	// Meta 协议专属配置及业务元数据，由各 Downloader 按需读取
	// HTTP 专属：
	//   "method"  string            — 默认 GET
	//   "headers" map[string]string — 请求头
	//   "body"    io.Reader         — 请求体
	Meta map[string]any
}

func NewTask(uri string, opts *Opts) *Task {
	if opts == nil {
		opts = &Opts{}
	}
	return &Task{URI: uri, Opts: opts}
}

func (t *Task) Interval() time.Duration {
	if t.Opts != nil {
		return t.Opts.Interval()
	}
	return time.Second
}

// ---------------------------------------------------------------------------
// Downloader 接口
// ---------------------------------------------------------------------------

type Downloader interface {
	// Name 返回协议标识，用于日志和注册。
	Name() string
	// CanHandle 根据 URI scheme（或 Meta 字段）判断能否处理该任务，不发起网络请求。
	CanHandle(task *Task) bool
	// Download 执行下载并将文件写到 task.Opts.Dest 路径。
	Download(ctx context.Context, task *Task) (*Result, error)
}
