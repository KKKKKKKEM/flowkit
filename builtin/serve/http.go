package serve

import (
	"net/http"

	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type HTTPConfig[Req, Resp any] struct {
	App      core.App[Req, Resp]
	BuildReq func(*gin.Context) (Req, error)
	OnStart  func(*gin.Context, *core.RunContext, Req)
}

func HTTP[Req, Resp any](r gin.IRouter, path string, cfg HTTPConfig[Req, Resp]) {
	buildReq := cfg.BuildReq
	if buildReq == nil {
		buildReq = defaultBuildReq[Req]
	}

	r.POST(path, func(c *gin.Context) {
		req, err := buildReq(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		rc := core.NewRunContext(c.Request.Context(), uuid.NewString())
		if cfg.OnStart != nil {
			cfg.OnStart(c, rc, req)
		}

		resp, err := cfg.App.Invoke(rc, req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	})
}
