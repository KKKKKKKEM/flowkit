package flowkit

import (
	"github.com/KKKKKKKEM/flowkit/cli"
	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/KKKKKKKEM/flowkit/server"
	"github.com/KKKKKKKEM/flowkit/server/sse"
	"github.com/gin-gonic/gin"
)

type serveConfig[Req, Resp any] struct {
	engine                   *gin.Engine
	path                     string
	store                    *sse.SessionStore
	builder                  func(*gin.Context) (Req, error)
	onStart                  func(*sse.Session, *core.Context, Req)
	disableTrackerProvider   bool
	disableInteractionPlugin bool
}

type ServeOption[Req, Resp any] func(*serveConfig[Req, Resp])

func WithEngine[Req, Resp any](e *gin.Engine) ServeOption[Req, Resp] {
	return func(c *serveConfig[Req, Resp]) { c.engine = e }
}

func WithPath[Req, Resp any](path string) ServeOption[Req, Resp] {
	return func(c *serveConfig[Req, Resp]) { c.path = path }
}

func WithStore[Req, Resp any](s *sse.SessionStore) ServeOption[Req, Resp] {
	return func(c *serveConfig[Req, Resp]) { c.store = s }
}

func WithServeBuilder[Req, Resp any](fn func(*gin.Context) (Req, error)) ServeOption[Req, Resp] {
	return func(c *serveConfig[Req, Resp]) { c.builder = fn }
}

func WithOnStart[Req, Resp any](fn func(*sse.Session, *core.Context, Req)) ServeOption[Req, Resp] {
	return func(c *serveConfig[Req, Resp]) { c.onStart = fn }
}

func DisableTrackerProvider[Req, Resp any]() ServeOption[Req, Resp] {
	return func(c *serveConfig[Req, Resp]) { c.disableTrackerProvider = true }
}

func DisableInteractionPlugin[Req, Resp any]() ServeOption[Req, Resp] {
	return func(c *serveConfig[Req, Resp]) { c.disableInteractionPlugin = true }
}

type cliConfig[Req, Resp any] struct {
	builder           func([]string) (Req, error)
	trackerProvider   core.TrackerProvider
	interactionPlugin core.InteractionPlugin
	onResult          func(Resp)
	onError           func(error)
	serveOpts         []ServeOption[Req, Resp]
}

type CLIOption[Req, Resp any] func(*cliConfig[Req, Resp])

func WithCLIBuilder[Req, Resp any](fn func([]string) (Req, error)) CLIOption[Req, Resp] {
	return func(c *cliConfig[Req, Resp]) { c.builder = fn }
}

func WithTrackerProvider[Req, Resp any](tp core.TrackerProvider) CLIOption[Req, Resp] {
	return func(c *cliConfig[Req, Resp]) { c.trackerProvider = tp }
}

func WithInteractionPlugin[Req, Resp any](ip core.InteractionPlugin) CLIOption[Req, Resp] {
	return func(c *cliConfig[Req, Resp]) { c.interactionPlugin = ip }
}

func WithOnResult[Req, Resp any](fn func(Resp)) CLIOption[Req, Resp] {
	return func(c *cliConfig[Req, Resp]) { c.onResult = fn }
}

func WithOnError[Req, Resp any](fn func(error)) CLIOption[Req, Resp] {
	return func(c *cliConfig[Req, Resp]) { c.onError = fn }
}

func WithServeOpts[Req, Resp any](opts ...ServeOption[Req, Resp]) CLIOption[Req, Resp] {
	return func(c *cliConfig[Req, Resp]) { c.serveOpts = append(c.serveOpts, opts...) }
}

type App[Req, Resp any] struct {
	invoke func(*core.Context, Req) (Resp, error)
}

func NewApp[Req, Resp any](invoke func(*core.Context, Req) (Resp, error)) App[Req, Resp] {
	return App[Req, Resp]{invoke: invoke}
}

func (a *App[Req, Resp]) Serve(addr string, opts ...ServeOption[Req, Resp]) error {
	cfg := &serveConfig[Req, Resp]{path: "/app"}
	for _, opt := range opts {
		opt(cfg)
	}

	engine := cfg.engine
	if engine == nil {
		engine = gin.Default()
	}

	server.SSE(engine, cfg.path, server.Config[Req, Resp]{
		App:                           core.AppFunc[Req, Resp](a.invoke),
		Store:                         cfg.store,
		Builder:                       cfg.builder,
		OnStart:                       cfg.onStart,
		DisableInnerTrackerProvider:   cfg.disableTrackerProvider,
		DisableInnerInteractionPlugin: cfg.disableInteractionPlugin,
	})
	return engine.Run(addr)
}

func (a *App[Req, Resp]) CLI(opts ...CLIOption[Req, Resp]) error {
	cfg := &cliConfig[Req, Resp]{}
	for _, opt := range opts {
		opt(cfg)
	}

	return cli.Run(cli.Config[Req, Resp]{
		App:               core.AppFunc[Req, Resp](a.invoke),
		Builder:           cfg.builder,
		Serve:             func(addr string) error { return a.Serve(addr, cfg.serveOpts...) },
		TrackerProvider:   cfg.trackerProvider,
		InteractionPlugin: cfg.interactionPlugin,
		OnResult:          cfg.onResult,
		OnError:           cfg.onError,
	})
}
