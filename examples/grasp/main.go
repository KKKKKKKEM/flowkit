package main

import (
	"log"

	"github.com/KKKKKKKEM/flowkit/x/grasp"
)

func main() {
	p := grasp.NewGraspPipeline()

	if err := p.Launch(); err != nil {
		log.Fatal(err)
	}
}
