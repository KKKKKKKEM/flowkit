package grasp

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/KKKKKKKEM/flowkit/stages/extract"
)

type CLIInteractionPlugin struct {
}

func (p *CLIInteractionPlugin) FormatResult(rc *core.Context, i core.Interaction, result *core.InteractionResult) (*core.InteractionResult, error) {
	var (
		indices []int
		err     error
	)
	items, ok := i.Payload.([]extract.Item)
	if !ok {
		return nil, fmt.Errorf("invalid interaction payload: expected []extract.Item, got %T", i.Payload)
	}

	line := strings.TrimSpace(fmt.Sprintf("%v", result.Answer))
	if line == "" || line == "all" {
		for i := 0; i < len(items); i++ {
			indices = append(indices, i)
		}
	} else {
		indices, err = parseCLIIndices(line, len(items))
		if err != nil {
			return nil, fmt.Errorf("select interact: invalid selection %q: %w", line, err)
		}
	}

	return &core.InteractionResult{Answer: indices}, nil
}

func (p *CLIInteractionPlugin) Interact(rc *core.Context, i core.Interaction) (*core.InteractionResult, error) {
	items, ok := i.Payload.([]extract.Item)
	if !ok {
		return nil, fmt.Errorf("invalid interaction payload: expected []extract.Item, got %T", i.Payload)
	}
	fmt.Println()
	for idx, item := range items {
		fmt.Printf("  [%d] %s\n", idx, item.Name)
		if item.URI != "" {
			fmt.Printf("      %s\n", item.URI)
		}
	}
	fmt.Println()
	fmt.Printf("Select items (e.g. 0,2 / 0-3 / all) [default: all]: ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("select interact: read input: %w", err)
	}
	return &core.InteractionResult{Answer: line}, nil
}

func parseCLIIndices(input string, total int) ([]int, error) {
	seen := make(map[int]struct{})
	var out []int

	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil || lo > hi {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			for i := lo; i <= hi; i++ {
				if i < 0 || i >= total {
					return nil, fmt.Errorf("index %d out of range [0, %d)", i, total)
				}
				if _, exists := seen[i]; !exists {
					seen[i] = struct{}{}
					out = append(out, i)
				}
			}
		} else {
			idx, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid index %q", part)
			}
			if idx < 0 || idx >= total {
				return nil, fmt.Errorf("index %d out of range [0, %d)", idx, total)
			}
			if _, exists := seen[idx]; !exists {
				seen[idx] = struct{}{}
				out = append(out, idx)
			}
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no valid indices")
	}
	return out, nil
}
