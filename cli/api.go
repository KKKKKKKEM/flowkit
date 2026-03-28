package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/google/uuid"
)

func Func[Req, Resp any](fn func(*core.Context, Req) (Resp, error)) core.App[Req, Resp] {
	return core.AppFunc[Req, Resp](fn)
}

type Config[Req, Resp any] struct {
	App               core.App[Req, Resp]
	Builder           func(args []string) (Req, error)
	Serve             func(addr string) error
	OnResult          func(resp Resp)
	OnError           func(err error)
	TrackerProvider   core.TrackerProvider
	InteractionPlugin core.InteractionPlugin
}

func Run[Req, Resp any](cfg Config[Req, Resp]) error {
	onResult := cfg.OnResult
	if onResult == nil {
		onResult = func(resp Resp) {
			enc := json.NewEncoder(os.Stdout)
			// enc.SetIndent("", "  ")
			_ = enc.Encode(resp)
		}
	}

	onError := cfg.OnError
	if onError == nil {
		onError = func(err error) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	if cfg.Serve != nil {
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		addr := fs.String("serve", "", "")
		fs.Usage = func() {}
		_ = fs.Parse(os.Args[1:])
		if *addr != "" {
			return cfg.Serve(*addr)
		}
	}

	req, err := cfg.Builder(os.Args[1:])
	if err != nil {
		onError(fmt.Errorf("build request: %w", err))
		return err
	}

	rc := core.NewContext(context.Background(), uuid.NewString())
	if cfg.TrackerProvider != nil {
		rc.WithTrackerProvider(cfg.TrackerProvider)
	}
	if cfg.InteractionPlugin != nil {
		rc.WithInteractionPlugin(cfg.InteractionPlugin)
	}

	resp, err := cfg.App.Invoke(rc, req)
	if err != nil {
		onError(err)
		return err
	}

	if cfg.TrackerProvider != nil {
		cfg.TrackerProvider.Wait()
	}

	onResult(resp)
	return nil
}
