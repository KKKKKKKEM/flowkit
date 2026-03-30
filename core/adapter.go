package core

import "fmt"

type TypedStageAdapter[In any, Out any] struct {
	name   string
	keyIn  string
	keyOut string
	stage  TypedStage[In, Out]
}

func (a *TypedStageAdapter[In, Out]) Name() string { return a.name }

func (a *TypedStageAdapter[In, Out]) Run(rc *Context) StageResult {
	in, ok := GetState[In](rc, a.keyIn)
	if !ok {
		return StageResult{
			Status: StageFailed,
			Err:    fmt.Errorf("missing input %q", a.keyIn),
		}
	}

	out, err := a.stage.Exec(rc, in)
	if err != nil {
		return StageResult{
			Status: StageFailed,
			Err:    err,
		}
	}

	return StageResult{
		Status: StageSuccess,
		Outputs: map[string]any{
			a.keyOut: out.Output,
		},
		Next:    out.Next,
		Metrics: out.Metrics,
	}
}

func NewTypedStage[In any, Out any](name string, keyIn string, keyOut string, stage TypedStage[In, Out]) *TypedStageAdapter[In, Out] {
	return &TypedStageAdapter[In, Out]{
		name:   name,
		keyIn:  keyIn,
		keyOut: keyOut,
		stage:  stage,
	}

}

func SetState[T any](rc *Context, key string, value T) {
	rc.State.Set(key, value)
}

func GetState[T any](rc *Context, key string) (T, bool) {
	var zero T
	v, ok := rc.State.Get(key)
	if !ok {
		return zero, false
	}
	out, ok := v.(T)
	if !ok {
		return zero, false
	}
	return out, true
}
