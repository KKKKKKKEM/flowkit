package serve

import "github.com/KKKKKKKEM/flowkit/core"

type SSEEventType string

const (
	SSEEventSession SSEEventType = "session"
	SSEProgress     SSEEventType = "progress"
	SSEInteract     SSEEventType = "interact"
	SSEDone         SSEEventType = "done"
	SSEError        SSEEventType = "error"
)

type SSEEvent struct {
	Seq  int64        `json:"seq"`
	Type SSEEventType `json:"type"`
	Data any          `json:"data"`
}

type InteractEventData struct {
	InteractionID string           `json:"interaction_id"`
	Interaction   core.Interaction `json:"interaction"`
}
