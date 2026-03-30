package server

import (
	"context"
	"net/http"
	"strconv"

	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/KKKKKKKEM/flowkit/server/sse"
	"github.com/gin-gonic/gin"
)

const (
	sessionIDKey   = "SESSION-ID"
	lastEventIDKey = "LAST-EVENT-ID"
)

type answerRequest struct {
	InteractionID string                 `json:"interaction_id"`
	Result        core.InteractionResult `json:"result"`
}

type Config[Req, Resp any] struct {
	App                           core.App[Req, Resp]
	Store                         *sse.SessionStore
	Builder                       func(*gin.Context) (Req, error)
	OnStart                       func(*sse.Session, *core.Context, Req)
	DisableInnerTrackerProvider   bool
	DisableInnerInteractionPlugin bool
}

func SSE[Req, Resp any](r gin.IRouter, path string, cfg Config[Req, Resp]) {
	store := cfg.Store
	if store == nil {
		store = sse.DefaultSSESessionStore()
	}
	builder := cfg.Builder
	if builder == nil {
		builder = DefaultBuildReq[Req]
	}

	r.POST(path+"/stream", func(c *gin.Context) {
		var (
			sseSession *sse.Session
			isNew      bool
			lastSeq    int64
			err        error
		)

		if v := c.GetHeader(lastEventIDKey); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				lastSeq = n
			}
		}

		sseSession, isNew, err = store.GetOrCreate(c.GetHeader(sessionIDKey))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		ch, unsub := sseSession.Subscribe(lastSeq)
		defer unsub()

		if isNew {
			req, err := builder(c)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			rc := core.NewContext(context.Background(), sseSession.ID)
			if !cfg.DisableInnerTrackerProvider {
				rc.Runtime.TrackerProvider = sse.NewSSETrackerProvider(sseSession)
			}

			if !cfg.DisableInnerInteractionPlugin {
				rc.Runtime.InteractionPlugin = sse.NewSSEInteractionPlugin(sseSession)

			}

			if cfg.OnStart != nil {
				cfg.OnStart(sseSession, rc, req)
			}

			go func() {

				resp, err := cfg.App.Invoke(rc, req)

				sseSession.Mu.Lock()
				sseSession.Done = true
				sseSession.Mu.Unlock()

				if err != nil {
					sseSession.Emit(sse.Error, gin.H{"message": err.Error()})
				} else {
					sseSession.Emit(sse.Done, resp)
				}
			}()
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		event := sse.Event{Seq: 0, Type: sse.EventSession, Data: gin.H{sessionIDKey: sseSession.ID}}
		event.Write(c)
		c.Writer.Flush()

		ctx := c.Request.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				e.Write(c)
				c.Writer.Flush()
				if e.Type == sse.Done || e.Type == sse.Error {
					store.Delete(sseSession.ID)
					return
				}
			}
		}
	})

	r.POST(path+"/answer", func(c *gin.Context) {
		sessionID := c.GetHeader(sessionIDKey)
		if sessionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Session-ID header required"})
			return
		}

		sess, ok := store.Get(sessionID)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}

		var body answerRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if err := sess.Answer(body.InteractionID, body.Result); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}
