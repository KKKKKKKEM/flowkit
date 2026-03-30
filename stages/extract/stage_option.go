package extract

type stageOptions struct {
	defaults      Opts
	nextStageName string
}

type Option func(*stageOptions)

func WithDefaults(opts *Opts) Option {
	return func(o *stageOptions) {
		if opts != nil {
			o.defaults = *opts.Clone()
		}
	}
}

func WithFallback(opts *Opts) Option {
	return WithDefaults(opts)
}

func WithNextStage(stageName string) Option {
	return func(o *stageOptions) { o.nextStageName = stageName }
}
