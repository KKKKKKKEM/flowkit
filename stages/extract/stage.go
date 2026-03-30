package extract

import (
	"fmt"
	"sort"

	"github.com/KKKKKKKEM/flowkit/core"
)

// Stage 通用解析 stage：
// 注册多个 Extractor，运行时根据 URL 匹配对应 Parser，输出 []Item
type Stage struct {
	*core.TypedStageAdapter[*Task, []Item]
	stageName  string
	opts       stageOptions
	extractors []Extractor
}

func (s *Stage) Exec(rc *core.Context, in *Task) (result core.TypedResult[[]Item], err error) {
	result.Next = s.opts.nextStageName

	applyFallback(in, &s.opts.fallback)
	parser := s.resolve(in.URL, in.ForcedParser)
	if parser == nil {
		err = fmt.Errorf("no parser matched URL: %s (forced: %s)", in.URL, in.ForcedParser)
		return
	}
	items, err := parser.Parse(rc, in, in.Opts)
	if err != nil {
		return
	}
	if in.OnItems != nil {
		in.OnItems(0, items)
	}
	result.Output = items

	return
}

func NewStage(name string, options ...Option) *Stage {
	s := &Stage{stageName: name}
	for _, opt := range options {
		opt(&s.opts)
	}
	inputKey := "task"
	outputKey := "items"
	s.TypedStageAdapter = core.NewTypedStage[*Task, []Item](
		name,
		inputKey,
		outputKey,
		s,
	)

	return s
}

// Mount 注册一个或多个 Extractor
func (s *Stage) Mount(extractors ...Extractor) *Stage {
	s.extractors = append(s.extractors, extractors...)
	return s
}

func (s *Stage) Name() string { return s.stageName }

// applyFallback 将 fb 中的非零值填充到 task.Opts，header 仅补充不覆盖。
func applyFallback(task *Task, fb *Opts) {
	if task.Opts == nil {
		task.Opts = &Opts{}
	}
	if task.Opts.Proxy == "" {
		task.Opts.Proxy = fb.Proxy
	}
	if task.Opts.Timeout == 0 {
		task.Opts.Timeout = fb.Timeout
	}
	if task.Opts.Retry == 0 {
		task.Opts.Retry = fb.Retry
	}
	if fb.Headers != nil {
		if task.Opts.Headers == nil {
			task.Opts.Headers = make(map[string]string)
		}
		for k, v := range fb.Headers {
			if _, exists := task.Opts.Headers[k]; !exists {
				task.Opts.Headers[k] = v
			}
		}
	}
}

func (s *Stage) resolve(rawURL, forcedHint string) *Parser {
	if forcedHint != "" {
		for _, ext := range s.extractors {
			for _, p := range ext.Handlers() {
				if p.Hint == forcedHint {
					return p
				}
			}
		}
		return nil // 指定了但找不到，返回 nil 报错
	}
	candidates := s.match(rawURL)
	if len(candidates) == 0 {
		return nil
	}
	return candidates[0]
}

// match 返回所有正则命中的 Parser，按 Priority 降序
func (s *Stage) match(rawURL string) []*Parser {
	var candidates []*Parser
	for _, ext := range s.extractors {
		for _, p := range ext.Handlers() {
			if p.Pattern != nil && p.Pattern.MatchString(rawURL) {
				candidates = append(candidates, p)
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Priority > candidates[j].Priority
	})
	return candidates
}
