package defaults

func OrZero[T comparable](current, fallback T) T {
	var zero T
	if current != zero {
		return current
	}
	return fallback
}

func MergeMap[K comparable, V any](base map[K]V, fallback map[K]V) map[K]V {
	if len(base) == 0 && len(fallback) == 0 {
		return nil
	}

	merged := make(map[K]V, max(len(base), len(fallback)))
	for k, v := range fallback {
		merged[k] = v
	}
	for k, v := range base {
		merged[k] = v
	}
	return merged
}

func MergeMapMissing[K comparable, V any](base map[K]V, fallback map[K]V) map[K]V {
	if len(base) == 0 && len(fallback) == 0 {
		return nil
	}

	merged := make(map[K]V, max(len(base), len(fallback)))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range fallback {
		if _, exists := merged[k]; !exists {
			merged[k] = v
		}
	}
	return merged
}
