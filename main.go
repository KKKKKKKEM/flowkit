package main

import (
	"context"
	"time"

	"github.com/KKKKKKKEM/grasp/pkg/core"
	"github.com/KKKKKKKEM/grasp/pkg/stage"
)

func main() {
	pipeline := core.NewPipeline()
	node := stage.NewDirectDownloadStage(
		&core.DownloadTask{
			URL:           "https://freemacsoft.net/downloads/AppCleaner_3.6.8.zip",
			Dest:          "AppCleaner_3.6.8.zip",
			Timeout:       30 * time.Second,
			Retry:         3,
			RetryInterval: 2 * time.Second,
		},
		stage.WithProgressBar(),
	)
	pipeline.Run(context.TODO(), node)
}
