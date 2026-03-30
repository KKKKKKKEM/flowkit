package flowkit

import (
	"flag"
	"fmt"
	"os"

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
	args              []string
	builder           func([]string) (Req, error)
	trackerProvider   core.TrackerProvider
	interactionPlugin core.InteractionPlugin
	onResult          func(Resp)
	onError           func(error)
}

type CLIOption[Req, Resp any] func(*cliConfig[Req, Resp])

func WithCLIBuilder[Req, Resp any](fn func([]string) (Req, error)) CLIOption[Req, Resp] {
	return func(c *cliConfig[Req, Resp]) { c.builder = fn }
}

func WithCLIArgs[Req, Resp any](args []string) CLIOption[Req, Resp] {
	return func(c *cliConfig[Req, Resp]) { c.args = args }
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

type LaunchMode string

const (
	LaunchModeCLI  LaunchMode = "cli"
	LaunchModeHTTP LaunchMode = "http"
)

type LaunchPlan struct {
	Mode LaunchMode
	Args []string
	Addr string
}

type launchConfig[Req, Resp any] struct {
	cli          cliConfig[Req, Resp]
	serve        serveConfig[Req, Resp]
	modeResolver func(args []string) (LaunchPlan, error)
	httpAddr     string
}

type LaunchOption[Req, Resp any] func(*launchConfig[Req, Resp])

func WithModeResolver[Req, Resp any](fn func(args []string) (LaunchPlan, error)) LaunchOption[Req, Resp] {
	return func(c *launchConfig[Req, Resp]) { c.modeResolver = fn }
}

func WithDefaultHTTPAddr[Req, Resp any](addr string) LaunchOption[Req, Resp] {
	return func(c *launchConfig[Req, Resp]) {
		if addr != "" {
			c.httpAddr = addr
		}
	}
}

func WithLaunchCLIOptions[Req, Resp any](opts ...CLIOption[Req, Resp]) LaunchOption[Req, Resp] {
	return func(c *launchConfig[Req, Resp]) {
		for _, opt := range opts {
			opt(&c.cli)
		}
	}
}

func WithLaunchServeOptions[Req, Resp any](opts ...ServeOption[Req, Resp]) LaunchOption[Req, Resp] {
	return func(c *launchConfig[Req, Resp]) {
		for _, opt := range opts {
			opt(&c.serve)
		}
	}
}

func defaultModeResolver(defaultHTTPAddr string) func(args []string) (LaunchPlan, error) {
	return func(args []string) (LaunchPlan, error) {
		if len(args) == 0 {
			return LaunchPlan{Mode: LaunchModeCLI, Args: nil}, nil
		}

		switch args[0] {
		case "serve":
			fs := flag.NewFlagSet("serve", flag.ContinueOnError)
			addr := fs.String("addr", defaultHTTPAddr, "")
			fs.Usage = func() {}
			if err := fs.Parse(args[1:]); err != nil {
				return LaunchPlan{}, err
			}
			return LaunchPlan{Mode: LaunchModeHTTP, Addr: *addr, Args: fs.Args()}, nil
		case "run":
			return LaunchPlan{Mode: LaunchModeCLI, Args: args[1:]}, nil
		default:
			return LaunchPlan{Mode: LaunchModeCLI, Args: args}, nil
		}
	}
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
		Args:              cfg.args,
		Builder:           cfg.builder,
		TrackerProvider:   cfg.trackerProvider,
		InteractionPlugin: cfg.interactionPlugin,
		OnResult:          cfg.onResult,
		OnError:           cfg.onError,
	})
}

func serveOptionsFromConfig[Req, Resp any](cfg serveConfig[Req, Resp]) []ServeOption[Req, Resp] {
	var opts []ServeOption[Req, Resp]
	if cfg.engine != nil {
		opts = append(opts, WithEngine[Req, Resp](cfg.engine))
	}
	if cfg.path != "" {
		opts = append(opts, WithPath[Req, Resp](cfg.path))
	}
	if cfg.store != nil {
		opts = append(opts, WithStore[Req, Resp](cfg.store))
	}
	if cfg.builder != nil {
		opts = append(opts, WithServeBuilder[Req, Resp](cfg.builder))
	}
	if cfg.onStart != nil {
		opts = append(opts, WithOnStart[Req, Resp](cfg.onStart))
	}
	if cfg.disableTrackerProvider {
		opts = append(opts, DisableTrackerProvider[Req, Resp]())
	}
	if cfg.disableInteractionPlugin {
		opts = append(opts, DisableInteractionPlugin[Req, Resp]())
	}
	return opts
}

func (a *App[Req, Resp]) Launch(opts ...LaunchOption[Req, Resp]) error {
	cfg := &launchConfig[Req, Resp]{httpAddr: ":8080", serve: serveConfig[Req, Resp]{path: "/app"}}
	for _, opt := range opts {
		opt(cfg)
	}

	args := cfg.cli.args
	if args == nil {
		args = os.Args[1:]
	}

	resolver := cfg.modeResolver
	if resolver == nil {
		resolver = defaultModeResolver(cfg.httpAddr)
	}

	plan, err := resolver(args)
	if err != nil {
		return err
	}

	switch plan.Mode {
	case LaunchModeHTTP:
		addr := plan.Addr
		if addr == "" {
			addr = cfg.httpAddr
		}
		return a.Serve(addr, serveOptionsFromConfig(cfg.serve)...)
	case LaunchModeCLI:
		return cli.Run(cli.Config[Req, Resp]{
			App:               core.AppFunc[Req, Resp](a.invoke),
			Args:              plan.Args,
			Builder:           cfg.cli.builder,
			TrackerProvider:   cfg.cli.trackerProvider,
			InteractionPlugin: cfg.cli.interactionPlugin,
			OnResult:          cfg.cli.onResult,
			OnError:           cfg.cli.onError,
		})
	default:
		return fmt.Errorf("unsupported launch mode: %s", plan.Mode)
	}
}
