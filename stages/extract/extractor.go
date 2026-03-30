package extract

import (
	"context"
	"regexp"
	"time"

	"github.com/KKKKKKKEM/flowkit/stages/download"
)

type Opts struct {

	// Proxy 指定下载使用的代理地址，支持 http://、https://、socks5:// 格式。
	// 特殊值 "env" 表示自动读取系统环境变量（HTTP_PROXY / HTTPS_PROXY / NO_PROXY）。
	Proxy string `json:"proxy,omitempty"`

	// Timeout 为单次 HTTP 请求（含 HEAD probe）的超时时间。0 表示不限制。
	Timeout time.Duration `json:"timeout,omitempty"`
	// Retry 为下载失败时的最大重试次数（不含首次），0 表示不重试。
	Retry   int               `json:"retry,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`

	// Meta 可选的元信息字段，供 Downloader 使用
	Meta map[string]any `json:"meta,omitempty"`
}

func (o *Opts) ToDownloaderOpts() *download.Opts {
	return &download.Opts{
		Proxy:   o.Proxy,
		Timeout: o.Timeout,
		Retry:   o.Retry,
	}

}

// Item 代表一个可下载的条目（详情页 or 列表项）
type Item struct {
	Name     string         `json:"name,omitempty"`
	URI      string         `json:"uri,omitempty"`       // 直链 or 页面 URI
	IsDirect bool           `json:"is_direct,omitempty"` // true = 直接下载链接，false = 需继续解析
	Meta     map[string]any `json:"meta,omitempty"`      // 封面、时长、分辨率等业务元数据
}

type Task struct {
	*Opts        `json:"*_opts,omitempty"`
	URL          string                        `json:"url,omitempty"`           // 入口 URL
	ForcedParser string                        `json:"forced_parser,omitempty"` // 可选：跳过 Match，直接指定解析器名
	MaxRounds    int                           `json:"max_rounds,omitempty"`    // 可选：多轮解析深度上限（0 用 Stage 默认值）
	OnItems      func(round int, items []Item) `json:"on_items,omitempty"`
}

func (t *Task) CloneWithURL(url string) *Task {
	optsCopy := *t.Opts // Opts 是值类型字段的结构体，直接值拷贝
	// Headers 是 map，需要独立拷贝（避免并发写）
	if t.Opts.Headers != nil {
		optsCopy.Headers = make(map[string]string, len(t.Opts.Headers))
		for k, v := range t.Opts.Headers {
			optsCopy.Headers[k] = v
		}
	}
	return &Task{
		Opts:         &optsCopy,
		URL:          url,
		ForcedParser: t.ForcedParser,
		MaxRounds:    t.MaxRounds,
		OnItems:      t.OnItems,
	}
}

// Extractor — Handler 的容器 + 命名空间
type Extractor interface {
	Name() string
	Handlers() []*Parser
}

// Parser — 真正的最小解析单元
type Parser struct {
	// Pattern 用于自动匹配，支持正则
	Pattern  *regexp.Regexp
	Priority int
	Hint     string // 语义标注，如 "search"、"detail"、"playlist"，用于日志

	Parse func(ctx context.Context, task *Task, opts *Opts) ([]Item, error)
}
