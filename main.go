package main

import (
	"context"
	"time"

	"github.com/KKKKKKKEM/grasp/pkg/core"
	"github.com/KKKKKKKEM/grasp/pkg/downloader"
	"github.com/KKKKKKKEM/grasp/pkg/stage"
)

func main() {
	pipeline := core.NewPipeline()
	task, err := downloader.NewTaskFromURI(
		context.TODO(),
		"https://videos.pexels.com/video-files/3929620/3929620-hd_1920_1080_30fps.mp4",
		&downloader.Opts{
			Timeout:       30 * time.Second,
			Retry:         3,
			RetryInterval: 2 * time.Second,
			Overwrite:     true,
			Concurrency:   2,
		},
		nil,
	)
	if err != nil {
		panic(err)
	}

	node := stage.NewDirectDownloadStage(
		task,
		stage.WithProgressBar(),
	)
	pipeline.Run(context.TODO(), node)
}
