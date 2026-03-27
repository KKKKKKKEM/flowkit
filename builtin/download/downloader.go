package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Segment struct {
	Idx     int   `json:"idx"`
	Start   int64 `json:"start"`
	End     int64 `json:"end"`     // inclusive; -1 means stream to EOF
	Written int64 `json:"written"` // bytes successfully written by consumer; R from Start+Written
	Done    bool  `json:"done"`
}

type Meta struct {
	TotalSize int64     `json:"total_size"`
	ChunkSize int64     `json:"chunk_size"`
	Segments  []Segment `json:"segments"`
}

// CompleteFunc 在下载成功后调用，result 包含实际文件路径和写入字节数。
type CompleteFunc func(result *Result)

// ProgressFunc 报告下载进度；total 为 -1 表示总大小未知。
type ProgressFunc func(downloaded, total int64)

// ErrorFunc 在下载失败时调用，err 为实际错误原因。
type ErrorFunc func(err error)

type Result struct {
	Path string `json:"path,omitempty"`
	Size int64  `json:"size,omitempty"`
}

type Opts struct {
	Dest string `json:"dest,omitempty"`

	// Proxy 指定下载使用的代理地址，支持 http://、https://、socks5:// 格式。
	// 特殊值 "env" 表示自动读取系统环境变量（HTTP_PROXY / HTTPS_PROXY / NO_PROXY）。
	Proxy string `json:"proxy,omitempty"`

	// Timeout 为单次 HTTP 请求（含 HEAD probe）的超时时间。0 表示不限制。
	Timeout time.Duration `json:"timeout,omitempty"`
	// Retry 为下载失败时的最大重试次数（不含首次），0 表示不重试。
	Retry int `json:"retry,omitempty"`
	// RetryInterval 为相邻两次重试之间的等待时间，默认 1s。
	RetryInterval time.Duration `json:"retry_interval,omitempty"`

	// Overwrite 控制目标文件已存在时的行为，默认为 false（跳过）。
	// 设为 true 时，无论是否已下载完成，均删除旧文件并重新下载。
	// 当 Overwrite 为 false 且文件已完整存在（且 R 为 false 或服务端不支持续传）时直接跳过。
	Overwrite bool `json:"overwrite,omitempty"`

	Concurrency int `json:"concurrency,omitempty"` // 下载并发数，默认为 1，表示单线程下载。大于 1 时启用分块下载。
	// ChunkSize 为分块下载时每个分片的字节数，0 表示使用默认值（1MB）。
	ChunkSize int64 `json:"chunk_size,omitempty"`

	SavePath string `json:"save_path,omitempty"` // 内部使用的实际保存路径，GetSavePath 计算后缓存
}

type Task struct {
	*Opts
	Request *http.Request
	// 进度回调, 下载进度
	OnProgress ProgressFunc
	// 完成回调, 下载成功后调用
	OnComplete CompleteFunc
	// 错误回调, 下载失败时调用
	OnError ErrorFunc
	// Meta 可选的元信息字段，供 Downloader 使用
	Meta map[string]any
}

// NewTaskFromURI 根据 URI 快速构造下载任务。
// opts 为空时会使用默认配置；headers 可选。
func NewTaskFromURI(uri string, opts *Opts, headers map[string]string) (*Task, error) {
	if strings.TrimSpace(uri) == "" {
		return nil, fmt.Errorf("uri is empty")
	}
	req, err := NewRequest(http.MethodGet, uri, headers)
	if err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &Opts{}
	}
	return &Task{
		Opts:    opts,
		Request: req,
	}, nil
}

func (t *Task) Interval() time.Duration {
	if t.RetryInterval > 0 {
		return t.RetryInterval
	}
	return time.Second
}

func (t *Task) MetaPath() (string, error) {
	if t.SavePath == "" {
		return "", fmt.Errorf("save path not resolved")
	}
	return t.SavePath + ".meta", nil
}

func (t *Task) LoadMeta() (*Meta, error) {
	metaPath, err := t.MetaPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var m Meta
	if err = json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (t *Task) SaveMeta(m *Meta) error {
	metaPath, err := t.MetaPath()
	if err != nil {
		return err
	}

	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0o644)
}

func (t *Task) RemoveMeta() error {
	metaPath, err := t.MetaPath()
	if err != nil {
		return err
	}
	return os.Remove(metaPath)
}

type Downloader interface {
	// CanHandle 根据 task 的任意字段综合判断，不应发起网络请求。
	CanHandle(task *Task) bool
	Download(ctx context.Context, task *Task) (*Result, error)
	Fetch(ctx context.Context, task *Task) (*http.Response, error)
	Name() string
}

func NewRequest(method, rawURL string, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req, nil
}
