package grasp

import (
	"github.com/KKKKKKKEM/flowkit/builtin/serve"
	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/gin-gonic/gin"
)

func (p *Pipeline) GinRegister(r gin.IRouter) gin.IRouter {
	store := serve.DefaultSSESessionStore()
	app := core.AppFunc[*Task, *Report](p.Invoke)
	serve.SSE(r, "/run", app, store,
		func(c *gin.Context) (*Task, error) {
			var task Task
			if err := c.ShouldBindJSON(&task); err != nil {
				return nil, err
			}
			return &task, nil
		},
		func(sess *serve.SSESession, rc *core.RunContext, _ *Task) {
			rc.WithReporter(NewSSEReporter(sess))
		},
	)
	return r
}

func (p *Pipeline) Serve(addr string) error {
	engine := gin.Default()
	p.GinRegister(engine)
	return engine.Run(addr)
}
