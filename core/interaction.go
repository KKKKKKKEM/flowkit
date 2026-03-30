package core

import "fmt"

type InteractionType string

const (
	InteractionTypeUserInput InteractionType = "user_input"
	InteractionTypeApproval  InteractionType = "approval"
	InteractionTypeCaptcha   InteractionType = "captcha"
	InteractionTypeCustom    InteractionType = "custom"
	InteractionTypeSelect    InteractionType = "select"
)

type Interaction struct {
	Type    InteractionType `json:"type"`
	Payload any             `json:"payload"`
	Message string          `json:"message,omitempty"`
}

type InteractionResult struct {
	Answer any `json:"answer"`
}

type ErrInteractionRequired struct {
	Interaction Interaction
}

func (e *ErrInteractionRequired) Error() string {
	return fmt.Sprintf("interaction required: type=%s", e.Interaction.Type)
}

type InteractionPlugin interface {
	Interact(rc *Context, i Interaction) (*InteractionResult, error)
	FormatResult(rc *Context, i Interaction, result *InteractionResult) (*InteractionResult, error)
}
