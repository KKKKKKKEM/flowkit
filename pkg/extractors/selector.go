package extractors

import "context"

type Selector interface {
	Select(ctx context.Context, items []ParseItem) ([]ParseItem, error)
}

type AllSelector struct{}

func (AllSelector) Select(_ context.Context, items []ParseItem) ([]ParseItem, error) {
	return items, nil
}

type FirstNSelector struct{ N int }

func (s FirstNSelector) Select(_ context.Context, items []ParseItem) ([]ParseItem, error) {
	if s.N >= len(items) {
		return items, nil
	}
	return items[:s.N], nil
}

type IndicesSelector struct{ Indices []int }

func (s IndicesSelector) Select(_ context.Context, items []ParseItem) ([]ParseItem, error) {
	var selected []ParseItem
	for _, idx := range s.Indices {
		if idx >= 0 && idx < len(items) {
			selected = append(selected, items[idx])
		}
	}
	return selected, nil
}
