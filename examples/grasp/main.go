package main

import (
	"log"

	"github.com/KKKKKKKEM/flowkit/x/download"
	"github.com/KKKKKKKEM/flowkit/x/extract"
	"github.com/KKKKKKKEM/flowkit/x/grasp"
	"github.com/KKKKKKKEM/flowkit/x/grasp/sites/pexels"
)

func main() {
	trackerProvider := grasp.NewMPBTrackerProvider()

	extractor := extract.NewStage("extractor")
	extractor.Mount(&pexels.APIParser{})

	downloader := download.NewStage("download")

	p := grasp.NewGraspPipeline(
		grasp.WithExtractor(extractor),
		grasp.WithDownloader(downloader),
		grasp.WithPlugin(&grasp.CLIInteractionPlugin{}),
		grasp.WithTrackerProvider(trackerProvider),
	)

	p.CLI()
	if err := p.Serve(":8080"); err != nil {
		log.Fatal(err)
	}
}
