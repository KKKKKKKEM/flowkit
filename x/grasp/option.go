package grasp

import (
	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/KKKKKKKEM/flowkit/stages/download"
	"github.com/KKKKKKKEM/flowkit/stages/extract"
)

type Option func(*Pipeline)

func WithExtractor(e *extract.Stage) Option {
	return func(p *Pipeline) {
		p.extractor = e
	}
}

func WithDownloader(d *download.Stage) Option {
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

func WithTrackerProvider(r core.TrackerProvider) Option {
	return func(p *Pipeline) {
		p.trackerProvider = r
	}
}

func WithPlugin(plugin core.InteractionPlugin) Option {
	return func(p *Pipeline) {
		p.interactionPlugin = plugin
	}
}

func WithExtractors(extractors ...extract.Extractor) Option {
	return func(p *Pipeline) {
		p.extractor.Mount(extractors...)
	}
}
