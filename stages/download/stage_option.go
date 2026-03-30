package download

type BatchFailStrategy int

const (
	BatchFailFast BatchFailStrategy = iota
	BatchBestEffort
)

type stageOptions struct {
	defaults         Opts
	nextStageName    string
	extraDownloaders []Downloader
	failStrategy     BatchFailStrategy
	maxConcurrency   int
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

func WithDownloaders(downloaders ...Downloader) Option {
	return func(o *stageOptions) {
		o.extraDownloaders = append(o.extraDownloaders, downloaders...)
	}
}

func WithFailStrategy(strategy BatchFailStrategy) Option {
	return func(o *stageOptions) { o.failStrategy = strategy }
}

func WithMaxConcurrency(n int) Option {
	return func(o *stageOptions) {
		if n > 0 {
			o.maxConcurrency = n
		}
	}
}
