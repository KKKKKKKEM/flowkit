package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	sessionIDKey   = "SESSION-ID"
	lastEventIDKey = "Last-Event-ID"
)

type answerRequest struct {
	InteractionID string                 `json:"interaction_id"`
	Result        core.InteractionResult `json:"result"`
}

type SSEConfig[Req, Resp any] struct {
	App      core.App[Req, Resp]
	Store    *SSESessionStore
	BuildReq func(*gin.Context) (Req, error)
	OnStart  func(*SSESession, *core.RunContext, Req)
}

func SSE[Req, Resp any](r gin.IRouter, path string, cfg SSEConfig[Req, Resp]) {
	store := cfg.Store
	if store == nil {
		store = DefaultSSESessionStore()
	}
	buildReq := cfg.BuildReq
	if buildReq == nil {
		buildReq = defaultBuildReq[Req]
	}

	r.POST(path, func(c *gin.Context) {
		lastSeq := int64(0)
		if v := c.GetHeader(lastEventIDKey); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				lastSeq = n
			}
		}

		sessionID := c.GetHeader(sessionIDKey)

		var (
			sess   *SSESession
			exists bool
		)
		if sessionID != "" {
			sess, exists = store.Get(sessionID)
		}
		if sess == nil {
			sessionID = uuid.NewString()
			sess = store.Create(sessionID)
			exists = false
		}

		ch, unsub := sess.subscribe(lastSeq)
		defer unsub()

		if !exists {
			req, err := buildReq(c)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			go func() {
				rc := core.NewRunContext(context.Background(), sessionID)
				rc.WithSuspend(func(i core.Interaction) (core.InteractionResult, error) {
					return sess.suspend(uuid.NewString(), i)
				})

				if cfg.OnStart != nil {
					cfg.OnStart(sess, rc, req)
				}

				resp, err := cfg.App.Invoke(rc, req)

				sess.mu.Lock()
				sess.done = true
				sess.mu.Unlock()

				if err != nil {
					sess.emit(SSEError, gin.H{"message": err.Error()})
				} else {
					sess.emit(SSEDone, resp)
				}
			}()
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		writeSSEEvent(c, SSEEvent{Seq: 0, Type: SSEEventSession, Data: gin.H{"session_id": sessionID}})
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
				writeSSEEvent(c, e)
				c.Writer.Flush()
				if e.Type == SSEDone || e.Type == SSEError {
					store.Delete(sessionID)
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

func writeSSEEvent(c *gin.Context, e SSEEvent) {
	b, _ := json.Marshal(e.Data)
	fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", e.Seq, e.Type, b)
}
