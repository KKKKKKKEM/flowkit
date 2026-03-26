package extractor

import (
	"fmt"
	"sort"

	"github.com/KKKKKKKEM/grasp/pkg/core"
	"github.com/KKKKKKKEM/grasp/pkg/extractors"
)

// Stage 通用解析 stage：
// 注册多个 Extractor，运行时根据 URL 匹配对应 Parser，输出 []extractor.ParseItem
type Stage struct {
	stageName  string
	opts       stageOptions
	extractors []extractors.Extractor
}

func NewStage(name string, options ...Option) *Stage {
	s := &Stage{stageName: name}
	for _, opt := range options {
		opt(&s.opts)
	}
	return s
}

// Mount 注册一个或多个 Extractor
func (s *Stage) Mount(extractors ...extractors.Extractor) *Stage {
	s.extractors = append(s.extractors, extractors...)
	return s
}

func (s *Stage) Name() string { return s.stageName }

func (s *Stage) Run(rc *core.RunContext) core.StageResult {

	var task *extractors.Task
	inputKey := s.opts.inputKey
	if inputKey == "" {
		inputKey = "task"
	}

	if val, ok := rc.Values[inputKey]; ok {
		if t, ok := val.(*extractors.Task); ok {
			task = t
		}
	}

	if task == nil {
		return core.StageResult{
			Status: core.StageFailed,
			Err:    fmt.Errorf("task not found: neither in rc.Inputs[\"%s\"] nor in stage default", inputKey),
		}
	}

	rawURL := task.URL

	// 收集所有匹配当前 URL 的 Parser，按 Priority 降序排列
	matched := s.match(rawURL)
	if len(matched) == 0 {
		return core.StageResult{
			Status: core.StageFailed,
			Err:    fmt.Errorf("no parser matched URL: %s", rawURL),
		}
	}

	parser := matched[0] // 最高优先级

	// 如果上游通过 rc.Values 注入了 Selector，透传给 task
	if sel, ok := rc.Values["selector"].(extractors.Selector); ok {
		task.Selector = sel
	}

	items, err := parser.Parse(rc, task, task.Opts)
	if err != nil {
		return core.StageResult{
			Status: core.StageFailed,
			Err:    fmt.Errorf("[%s] parse failed: %w", parser.Hint, err),
		}
	}

	return core.StageResult{
		Status: core.StageSuccess,
		Next:   s.opts.nextStageName,
		Outputs: map[string]any{
			"items": items,
		},
	}
}

// match 返回所有正则命中的 Parser，按 Priority 降序
func (s *Stage) match(rawURL string) []*extractors.Parser {
	var candidates []*extractors.Parser
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
