package extractors

import (
	"context"
	"regexp"
	"time"

	"github.com/KKKKKKKEM/grasp/pkg/downloader"
)

type Opts struct {

	// Proxy 指定下载使用的代理地址，支持 http://、https://、socks5:// 格式。
	// 特殊值 "env" 表示自动读取系统环境变量（HTTP_PROXY / HTTPS_PROXY / NO_PROXY）。
	Proxy string

	// Timeout 为单次 HTTP 请求（含 HEAD probe）的超时时间。0 表示不限制。
	Timeout time.Duration
	// Retry 为下载失败时的最大重试次数（不含首次），0 表示不重试。
	Retry   int
	Headers map[string]string

	// Meta 可选的元信息字段，供 Downloader 使用
	Meta map[string]any
}

func (o *Opts) ToDownloaderOpts() *downloader.Opts {
	return &downloader.Opts{
		Proxy:   o.Proxy,
		Timeout: o.Timeout,
		Retry:   o.Retry,
	}

}

// ParseItem 代表一个可下载的条目（详情页 or 列表项）
type ParseItem struct {
	Name     string
	URI      string         // 直链 or 页面 URI
	IsDirect bool           // true = 直接下载链接，false = 需继续解析
	Meta     map[string]any // 封面、时长、分辨率等业务元数据
}

type Task struct {
	*Opts
	URL          string   // 入口 URL
	ForcedParser string   // 可选：跳过 Match，直接指定解析器名
	Selector     Selector // 可选：为这个任务指定选择器（nil 则用 Stage 默认值）
	MaxRounds    int      // 可选：多轮解析深度上限（0 用 Stage 默认值）
	OnItems      func(round int, items []ParseItem)
}

type Selector interface {
	Select(ctx context.Context, items []ParseItem) ([]ParseItem, error)
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

	Parse func(ctx context.Context, task *Task, opts *Opts) ([]ParseItem, error)
}
