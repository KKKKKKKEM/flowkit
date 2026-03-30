package extract

import "github.com/KKKKKKKEM/flowkit/stages/internal/defaults"

func (o *Opts) Clone() *Opts {
	if o == nil {
		return &Opts{}
	}
	clone := *o
	clone.Headers = defaults.MergeMap(o.Headers, nil)
	if o.Meta != nil {
		clone.Meta = make(map[string]any, len(o.Meta))
		for k, v := range o.Meta {
			clone.Meta[k] = v
		}
	}
	return &clone
}

func ResolveOpts(base *Opts, fallback *Opts) *Opts {
	resolved := base.Clone()
	if fallback == nil {
		return resolved
	}

	resolved.Proxy = defaults.OrZero(resolved.Proxy, fallback.Proxy)
	resolved.Timeout = defaults.OrZero(resolved.Timeout, fallback.Timeout)
	resolved.Retry = defaults.OrZero(resolved.Retry, fallback.Retry)
	resolved.Headers = defaults.MergeMapMissing(resolved.Headers, fallback.Headers)
	if len(resolved.Meta) == 0 && len(fallback.Meta) > 0 {
		resolved.Meta = make(map[string]any, len(fallback.Meta))
		for k, v := range fallback.Meta {
			resolved.Meta[k] = v
		}
	}
	return resolved
}

func (t *Task) Clone() *Task {
	if t == nil {
		return nil
	}
	clone := *t
	clone.Opts = t.Opts.Clone()
	return &clone
}
