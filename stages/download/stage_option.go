package download

type stageOptions struct {
	fallback         Opts
	nextStageName    string
	extraDownloaders []Downloader
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

func WithDownloaders(downloaders ...Downloader) Option {
	return func(o *stageOptions) {
		o.extraDownloaders = append(o.extraDownloaders, downloaders...)
	}
}
