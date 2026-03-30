package download

import "github.com/KKKKKKKEM/flowkit/stages/internal/defaults"

func (o *Opts) Clone() *Opts {
	if o == nil {
		return &Opts{}
	}
	clone := *o
	return &clone
}

func ResolveOpts(base *Opts, fallback *Opts) *Opts {
	resolved := base.Clone()
	if fallback == nil {
		return resolved
	}

	resolved.Dest = defaults.OrZero(resolved.Dest, fallback.Dest)
	resolved.Proxy = defaults.OrZero(resolved.Proxy, fallback.Proxy)
	resolved.Timeout = defaults.OrZero(resolved.Timeout, fallback.Timeout)
	resolved.Retry = defaults.OrZero(resolved.Retry, fallback.Retry)
	resolved.RetryInterval = defaults.OrZero(resolved.RetryInterval, fallback.RetryInterval)
	resolved.Concurrency = defaults.OrZero(resolved.Concurrency, fallback.Concurrency)
	resolved.ChunkSize = defaults.OrZero(resolved.ChunkSize, fallback.ChunkSize)
	return resolved
}

func (t *Task) CloneWithOpts(opts *Opts) *Task {
	if t == nil {
		return nil
	}

	clone := *t
	clone.Opts = opts.Clone()
	if t.Meta != nil {
		clone.Meta = make(map[string]any, len(t.Meta))
		for k, v := range t.Meta {
			clone.Meta[k] = v
		}
	}
	return &clone
}
