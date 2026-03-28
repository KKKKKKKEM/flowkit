package sse

import (
	"encoding/json"
	"fmt"

	"github.com/KKKKKKKEM/flowkit/core"
	"github.com/gin-gonic/gin"
)

type EventType string

const (
	EventSession EventType = "session"
	Track        EventType = "track"
	Interact     EventType = "interact"
	Done         EventType = "done"
	Error        EventType = "error"
)

type Event struct {
	Seq  int64     `json:"seq"`
	Type EventType `json:"type"`
	Data any       `json:"data"`
}

func (e Event) Write(c *gin.Context) {
	b, _ := json.Marshal(e.Data)
	fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", e.Seq, e.Type, b)
}

type InteractEventData struct {
	InteractionID string           `json:"interaction_id"`
	Interaction   core.Interaction `json:"interaction"`
}
