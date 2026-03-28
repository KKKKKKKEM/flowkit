package grasp

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/KKKKKKKEM/flowkit/cli"
)

func (p *Pipeline) CLI() error {
	return cli.Run(cli.Config[*Task, *Report]{
		App:               cli.Func(p.Invoke),
		BuildReq:          buildTaskFromArgs,
		TrackerProvider:   NewMPBTrackerProvider(),
		InteractionPlugin: &CLIInteractionPlugin{},
	})
}

func buildTaskFromArgs(args []string) (*Task, error) {
	fs := flag.NewFlagSet("grasp", flag.ContinueOnError)

	var (
		urls        string
		proxy       string
		timeout     time.Duration
		retry       int
		headers     string
		maxRounds   int
		concurrency int
		dest        string
		overwrite   bool
		dlConc      int
		chunkSize   int64
	)

	fs.StringVar(&urls, "url", "", "comma-separated URLs to grasp (required)")
	fs.StringVar(&proxy, "proxy", "", "proxy URL")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "request timeout")
	fs.IntVar(&retry, "retry", 3, "retry count")
	fs.StringVar(&headers, "header", "", "extra headers, format: Key:Value,Key2:Value2")
	fs.IntVar(&maxRounds, "rounds", 1, "max extract rounds")
	fs.IntVar(&concurrency, "concurrency", 1, "extract concurrency")
	fs.StringVar(&dest, "dest", ".", "download destination directory")
	fs.BoolVar(&overwrite, "overwrite", false, "overwrite existing files")
	fs.IntVar(&dlConc, "dl-concurrency", 3, "download concurrency")
	fs.Int64Var(&chunkSize, "chunk-size", 0, "download chunk size in bytes (0 = auto)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if urls == "" {
		return nil, fmt.Errorf("-url is required")
	}

	task := &Task{
		URLs:    splitTrimmed(urls, ","),
		Proxy:   proxy,
		Timeout: timeout,
		Retry:   retry,
		Extract: ExtractConfig{
			MaxRounds:   maxRounds,
			Concurrency: concurrency,
		},
		Download: DownloadConfig{
			Dest:        dest,
			Overwrite:   overwrite,
			Concurrency: dlConc,
			ChunkSize:   chunkSize,
		},
	}

	if headers != "" {
		task.Headers = parseHeaders(headers)
	}

	return task, nil
}

func splitTrimmed(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseHeaders(s string) map[string]string {
	m := make(map[string]string)
	for _, pair := range splitTrimmed(s, ",") {
		kv := strings.SplitN(pair, ":", 2)
		if len(kv) == 2 {
			m[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return m
}
