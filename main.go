package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/KKKKKKKEM/grasp/pkg/core"
	"github.com/KKKKKKKEM/grasp/pkg/core/pipeline"
	"github.com/KKKKKKKEM/grasp/pkg/extract"
	"github.com/KKKKKKKEM/grasp/pkg/extract/pexels"
	extractStage "github.com/KKKKKKKEM/grasp/pkg/stage/extract"
	"github.com/KKKKKKKEM/grasp/pkg/stage/http_download"
)

func main() {
	// 创建 Pipeline
	fsmPipeline := pipeline.NewFSMPipeline()

	// 创建 DirectDownloadStage（不指定默认 Task，运行时从 rc.Inputs 读取）
	downloadStage := http_download.NewStage(
		"download",
		http_download.WithProgressBar(),
	)

	extractorStage := extractStage.NewStage("extractor")
	extractorStage.Mount(&pexels.APIParser{})

	// 注册 stage
	fsmPipeline.Register(downloadStage, extractorStage)

	task := &extract.Task{
		Opts: &extract.Opts{},
		URL:  "https://api.pexels.com/v1/photos/1000",
	}

	rc := core.NewRunContext(context.Background(), "trace-001")
	rc.WithValue("task", task)
	report, err := fsmPipeline.Run(rc, "extractor")
	if err != nil {
		log.Fatalf("Pipeline failed: %v", err)
	}

	bytes, err := json.Marshal(report.StageResults)
	if err != nil {
		return
	}

	log.Printf("Pipeline %s completed in %dms, success=%v", report.Mode, report.DurationMs, report.Success)
	log.Printf("Execution order: %v", report.StageOrder)
	log.Printf("Execution result: %v", string(bytes))

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
	//report1, err := fsmPipeline.Run(rc1, "download")
	//if err != nil {
	//	log.Fatalf("Pipeline failed: %v", err)
	//}
	//
	//log.Printf("Pipeline %s completed in %dms, success=%v", report1.Mode, report1.DurationMs, report1.Success)
	//log.Printf("Execution order: %v", report1.StageOrder)

}
