package cli

import (
	"context"
	"encoding/json"
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
	Args              []string
	Builder           func(args []string) (Req, error)
	OnResult          func(resp Resp)
	OnError           func(err error)
	TrackerProvider   core.TrackerProvider
	InteractionPlugin core.InteractionPlugin
}

func Run[Req, Resp any](cfg Config[Req, Resp]) error {
	args := cfg.Args
	if args == nil {
		args = os.Args[1:]
	}

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

	req, err := cfg.Builder(args)
	if err != nil {
		onError(fmt.Errorf("build request: %w", err))
		return err
	}

	rc := core.NewContext(context.Background(), uuid.NewString())
	if cfg.TrackerProvider != nil {
		rc.Runtime.TrackerProvider = cfg.TrackerProvider
	}
	if cfg.InteractionPlugin != nil {
		rc.Runtime.InteractionPlugin = cfg.InteractionPlugin
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
