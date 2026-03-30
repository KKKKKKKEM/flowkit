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

	resolvedTask := in.Clone()
	resolvedTask.Opts = ResolveOpts(in.Opts, &s.opts.defaults)
	parser := s.resolve(resolvedTask.URL, resolvedTask.ForcedParser)
	if parser == nil {
		err = fmt.Errorf("no parser matched URL: %s (forced: %s)", resolvedTask.URL, resolvedTask.ForcedParser)
		return
	}
	items, err := parser.Parse(rc, resolvedTask, resolvedTask.Opts)
	if err != nil {
		return
	}
	if resolvedTask.OnItems != nil {
		resolvedTask.OnItems(0, items)
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
