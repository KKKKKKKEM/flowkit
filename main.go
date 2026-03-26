package main

import (
	"context"
	"log"

	"github.com/KKKKKKKEM/grasp/pkg/core"
	"github.com/KKKKKKKEM/grasp/pkg/extractors"
	"github.com/KKKKKKKEM/grasp/pkg/extractors/pexels"
	"github.com/KKKKKKKEM/grasp/pkg/stage/extractor"
	"github.com/KKKKKKKEM/grasp/pkg/stage/http_download"
)

func main() {
	// 创建 Pipeline
	pipeline := core.NewFSMPipeline()

	// 创建 DirectDownloadStage（不指定默认 Task，运行时从 rc.Inputs 读取）
	downloadStage := http_download.NewStage(
		"download",
		http_download.WithProgressBar(),
	)

	extractorStage := extractor.NewStage("extractor")
	extractorStage.Mount(&pexels.APIParser{})

	// 注册 stage
	pipeline.Register(downloadStage)
	pipeline.Register(extractorStage)

	task := &extractors.Task{
		Opts: &extractors.Opts{},
		URL:  "https://api.pexels.com/v1/photos/1000",
	}

	rc := core.NewRunContext(context.Background(), "trace-001")
	rc.WithValue("task", task)
	report, err := pipeline.Run(rc, "extractor")
	if err != nil {
		log.Fatalf("Pipeline failed: %v", err)
	}

	log.Printf("Pipeline %s completed in %dms, success=%v", report.Mode, report.DurationMs, report.Success)
	log.Printf("Execution order: %v", report.StageOrder)
	log.Printf("Execution result: %v", report.StageResults)

	//
	//
	//task1, err := downloader.NewTaskFromURI(
	//	"https://videos.pexels.com/video-files/3929620/3929620-hd_1920_1080_30fps.mp4",
	//	&downloader.Opts{
	//		Timeout:       30 * time.Second,
	//		Retry:         3,
	//		RetryInterval: 2 * time.Second,
	//		Overwrite:     true,
	//		Concurrency:   2,
	//	},
	//	nil,
	//)
	//if err != nil {
	//	panic(err)
	//}
	//
	//// 创建运行上下文，通过 Inputs 传递任务
	//rc1 := core.NewRunContext(context.Background(), "trace-001")
	//rc1.Inputs["task"] = task1 // ✨ 运行时输入
	//
	//report1, err := pipeline.Run(rc1, "download")
	//if err != nil {
	//	log.Fatalf("Pipeline failed: %v", err)
	//}
	//
	//log.Printf("Pipeline %s completed in %dms, success=%v", report1.Mode, report1.DurationMs, report1.Success)
	//log.Printf("Execution order: %v", report1.StageOrder)

}
