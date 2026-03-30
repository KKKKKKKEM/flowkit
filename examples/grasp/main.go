package main

import (
	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/KKKKKKKEM/flowkit/x/grasp"
)

func main() {
	p := grasp.NewGraspPipeline()

	//err := p.CLI()
	//if err != nil {
	//	log.Fatal(err)
	//}

	//if err := p.Serve(":8080"); err != nil {
	//	log.Fatal(err)
	//}

	p.Invoke(core.NewContext(nil, ""), &grasp.Task{
		URLs: []string{"https://api.pexels.com/v1/photos/1000"},
	})
}
