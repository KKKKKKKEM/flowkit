package grasp

import (
	"github.com/KKKKKKKEM/flowkit/builtin/serve"
	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/gin-gonic/gin"
)

func (p *Pipeline) GinRegister(r gin.IRouter) gin.IRouter {
	serve.SSE(r, "/run", serve.SSEConfig[*Task, *Report]{
		App: serve.Func(p.Invoke),
		OnStart: func(sess *serve.SSESession, rc *core.RunContext, _ *Task) {
			rc.WithReporter(NewSSEReporter(sess))
		},
	})
	return r
}

func (p *Pipeline) Serve(addr string) error {
	engine := gin.Default()
	p.GinRegister(engine)
	return engine.Run(addr)
}
