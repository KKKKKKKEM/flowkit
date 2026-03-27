package grasp

import (
	"github.com/KKKKKKKEM/flowkit/builtin/download"
	"github.com/KKKKKKKEM/flowkit/builtin/extract"
	"github.com/KKKKKKKEM/flowkit/core"
)

type Option func(*Pipeline)

func WithExtractor(e *extract.Stage) Option {
	return func(p *Pipeline) {
		p.extractor = e
	}
}

func WithDownloader(d *download.DirectDownloadStage) Option {
	return func(p *Pipeline) {
		p.downloader = d
	}
}

func WithSelector(fn SelectFunc) Option {
	return func(p *Pipeline) {
		p.defaultSelector = fn
	}
}

func WithTransform(fn TransformFunc) Option {
	return func(p *Pipeline) {
		p.defaultTransform = fn
	}
}

func WithProgress(r core.ProgressReporter) Option {
	return func(p *Pipeline) {
		p.reporter = r
	}
}

func WithPlugin(plugin core.InteractionPlugin) Option {
	return func(p *Pipeline) {
		p.plugin = plugin
	}
}
