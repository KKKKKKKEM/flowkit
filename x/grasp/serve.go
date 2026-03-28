package grasp

import (
	"github.com/KKKKKKKEM/flowkit/server"
	"github.com/gin-gonic/gin"
)

func (p *Pipeline) Serve(addr string) error {
	engine := gin.Default()
	server.SSE(engine, "/grasp", server.Config[*Task, *Report]{
		App: server.Func(p.Invoke),
	})
	return engine.Run(addr)
}
