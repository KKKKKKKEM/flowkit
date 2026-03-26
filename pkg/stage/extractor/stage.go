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

func (s *Stage) loadTask(rc *core.RunContext) (*extractors.Task, error) {

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
		return nil, fmt.Errorf("task not found: neither in rc.Inputs[\"%s\"] nor in stage default", inputKey)
	}

	return task, nil

}
func (s *Stage) resolve(rawURL, forcedHint string) *extractors.Parser {
	if forcedHint != "" {
		// 按 Extractor.Name() + Parser.Hint 精确指定
		for _, ext := range s.extractors {
			for _, p := range ext.Handlers() {
				if p.Hint == forcedHint {
					return p
				}
			}
		}
		return nil // 指定了但找不到，返回 nil 报错
	}
	// 回到正则 match 逻辑
	return s.match(rawURL)[0]
}

func (s *Stage) resolveSelector(rc *core.RunContext, task *extractors.Task) extractors.Selector {
	// 1. Task 级最高优先（调用方显式指定）
	if task.Selector != nil {
		return task.Selector
	}
	// 2. 运行时注入（上游 Stage 通过 rc.Values 传入）
	if sel, ok := rc.Values["selector"].(extractors.Selector); ok {
		return sel
	}
	// 3. Stage 构造时的默认值
	return s.opts.defaultSelector // 可以是 nil，调用方判空
}

func (s *Stage) Run(rc *core.RunContext) core.StageResult {

	task, err := s.loadTask(rc)
	if err != nil {
		return core.StageResult{
			Status: core.StageFailed,
			Err:    err,
		}
	}
	maxRounds := task.MaxRounds
	if maxRounds == 0 {
		maxRounds = s.opts.maxRounds // Stage 级默认值
	}
	if maxRounds == 0 {
		maxRounds = 1
	}

	var allDirect []extractors.ParseItem
	queue := []string{task.URL}
	for round := 0; round < maxRounds && len(queue) > 0; round++ {
		var nextQueue []string

		for _, rawURL := range queue {
			parser := s.resolve(rawURL, task.ForcedParser)
			if parser == nil {
				return core.StageResult{Status: core.StageFailed, Err: fmt.Errorf("no parser matched URL: %s (forced: %s)", rawURL, task.ForcedParser)}
			}

			subTask := task.CloneWithURL(rawURL)
			items, err := parser.Parse(rc, subTask, task.Opts)
			if err != nil {
				return core.StageResult{Status: core.StageFailed, Err: err}
			}

			if task.OnItems != nil {
				task.OnItems(round, items)
			}

			// Selector 在此处统一触发
			sel := s.resolveSelector(rc, task)
			if sel != nil {
				items, err = sel.Select(rc, items)
				if err != nil {
					return core.StageResult{Status: core.StageFailed, Err: err}
				}
			}

			for _, item := range items {
				if item.IsDirect {
					allDirect = append(allDirect, item)
				} else {
					nextQueue = append(nextQueue, item.URI) // 继续解析
				}
			}
		}
		queue = nextQueue
	}

	return core.StageResult{
		Status:  core.StageSuccess,
		Next:    s.opts.nextStageName,
		Outputs: map[string]any{"items": allDirect},
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
