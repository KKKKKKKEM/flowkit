package grasp

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

func buildCLI(args []string) (*Task, error) {
	fs := flag.NewFlagSet("grasp", flag.ContinueOnError)

	var (
		urls                string
		proxy               string
		timeout             time.Duration
		retry               int
		headers             string
		maxRounds           int
		extractConcurrency  int
		dest                string
		overwrite           bool
		downloadTaskConc    int
		bestEffort          bool
		downloadSegmentConc int
		chunkSize           int64
	)

	fs.StringVar(&urls, "url", "", "comma-separated URLs to grasp (required)")
	fs.StringVar(&proxy, "proxy", "", "proxy URL")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "request timeout")
	fs.IntVar(&retry, "retry", 3, "retry count")
	fs.StringVar(&headers, "header", "", "extra headers, format: Key:Value,Key2:Value2")
	fs.IntVar(&maxRounds, "rounds", 1, "max extract rounds")
	fs.IntVar(&extractConcurrency, "extract-concurrency", 1, "max concurrent extract workers")
	fs.StringVar(&dest, "dest", ".", "download destination directory")
	fs.BoolVar(&overwrite, "overwrite", false, "overwrite existing files")
	fs.IntVar(&downloadTaskConc, "download-task-concurrency", 0, "max concurrent download tasks (0 = no extra limit)")
	fs.BoolVar(&bestEffort, "best-effort", false, "continue downloading other tasks after individual failures")
	fs.IntVar(&downloadSegmentConc, "download-segment-concurrency", 3, "concurrent segments per download task")
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
			MaxRounds:         maxRounds,
			WorkerConcurrency: extractConcurrency,
		},
		Download: DownloadConfig{
			Dest:               dest,
			Overwrite:          overwrite,
			TaskConcurrency:    downloadTaskConc,
			BestEffort:         bestEffort,
			SegmentConcurrency: downloadSegmentConc,
			ChunkSize:          chunkSize,
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
