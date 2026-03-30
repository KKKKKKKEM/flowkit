package extract

type stageOptions struct {
	fallback      Opts
	nextStageName string
}

type Option func(*stageOptions)

func WithFallback(opts *Opts) Option {
	return func(o *stageOptions) {
		if opts != nil {
			o.fallback = *opts
		}
	}
}

func WithNextStage(stageName string) Option {
	return func(o *stageOptions) { o.nextStageName = stageName }
}
