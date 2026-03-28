package sse

import (
	"context"
	"net/http"
	"strconv"

	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/KKKKKKKEM/flowkit/server"
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
	Store                         *SessionStore
	BuildReq                      func(*gin.Context) (Req, error)
	OnStart                       func(*Session, *core.Context, Req)
	DisableInnerTrackerProvider   bool
	DisableInnerInteractionPlugin bool
}

func SSE[Req, Resp any](r gin.IRouter, path string, cfg Config[Req, Resp]) {
	store := cfg.Store
	if store == nil {
		store = DefaultSSESessionStore()
	}
	buildReq := cfg.BuildReq
	if buildReq == nil {
		buildReq = server.DefaultBuildReq[Req]
	}

	r.POST(path+"/stream", func(c *gin.Context) {
		var (
			sseSession *Session
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

		ch, unsub := sseSession.subscribe(lastSeq)
		defer unsub()

		if isNew {
			req, err := buildReq(c)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			rc := core.NewContext(context.Background(), sseSession.ID)
			if !cfg.DisableInnerTrackerProvider {
				rc.WithTrackerProvider(NewSSETrackerProvider(sseSession))
			}

			if !cfg.DisableInnerInteractionPlugin {
				rc.WithInteractionPlugin(NewSSEInteractionPlugin(sseSession))

			}

			if cfg.OnStart != nil {
				cfg.OnStart(sseSession, rc, req)
			}

			go func() {

				resp, err := cfg.App.Invoke(rc, req)

				sseSession.mu.Lock()
				sseSession.done = true
				sseSession.mu.Unlock()

				if err != nil {
					sseSession.Emit(Error, gin.H{"message": err.Error()})
				} else {
					sseSession.Emit(Done, resp)
				}
			}()
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		event := Event{Seq: 0, Type: EventSession, Data: gin.H{sessionIDKey: sseSession.ID}}
		event.write(c)
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
				e.write(c)
				c.Writer.Flush()
				if e.Type == Done || e.Type == Error {
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

		if err := sess.answer(body.InteractionID, body.Result); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}
