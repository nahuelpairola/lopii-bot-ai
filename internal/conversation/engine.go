package conversation

import (
	"fmt"
	"maps"
	"time"
)

type Result struct {
	Prompt   Prompt
	Finished bool
	FlowName string
	Data     Data
}

type stateStore interface {
	Get(userID uint64) (flowName, stepName string, data Data, updatedAt time.Time, found bool, err error)
	Set(userID uint64, flowName, stepName string, data Data) error
	Clear(userID uint64) error
}

const idleThreshold = 10 * time.Minute

const retryCountKey = "_retry_count"

const (
	resumeContinue = "_resume_continue"
	resumeCancel   = "_resume_cancel"
)

const ResumeCancelledKey = "_resume_cancelled"

type Engine struct {
	flows       map[string]*Flow
	store       stateStore
	resumeLabel func(flowName string) string
}

func NewEngine(store stateStore, resumeLabel func(flowName string) string) *Engine {
	return &Engine{flows: make(map[string]*Flow), store: store, resumeLabel: resumeLabel}
}

func (e *Engine) Register(f *Flow) {
	e.flows[f.Name] = f
}

const UserIDKey = "_user_id"

func (e *Engine) Start(userID uint64, flowName string) (Prompt, error) {
	return e.StartWithData(userID, flowName, Data{})
}

func (e *Engine) StartWithData(userID uint64, flowName string, seed Data) (Prompt, error) {
	f, ok := e.flows[flowName]
	if !ok {
		return Prompt{}, fmt.Errorf("conversation: flow %q is not registered", flowName)
	}

	data := Data{UserIDKey: userID}
	for k, v := range seed {
		data[k] = v
	}

	resolved, err := f.advanceThroughSkips(f.InitialStep, data)
	if err != nil {
		return Prompt{}, err
	}
	if resolved == "" {
		return Prompt{}, fmt.Errorf("conversation: flow %q completed immediately with seed data, nothing to prompt", flowName)
	}

	step, ok := f.step(resolved)
	if !ok {
		return Prompt{}, fmt.Errorf("conversation: step %q not found in flow %q", resolved, flowName)
	}
	if err := e.store.Set(userID, f.Name, resolved, data); err != nil {
		return Prompt{}, err
	}
	return step.Prompt(data), nil
}

func (e *Engine) InProgress(userID uint64) (bool, error) {
	_, _, _, _, found, err := e.store.Get(userID)
	return found, err
}

func (e *Engine) Clear(userID uint64) error {
	return e.store.Clear(userID)
}

func (e *Engine) Handle(userID uint64, input Input) (result Result, found bool, err error) {
	flowName, stepName, data, updatedAt, found, err := e.store.Get(userID)
	if err != nil || !found {
		return Result{}, found, err
	}

	if input.CallbackData == resumeContinue {
		f, ok := e.flows[flowName]
		if !ok {
			return Result{}, true, fmt.Errorf("conversation: flow %q is not registered", flowName)
		}
		step, ok := f.step(stepName)
		if !ok {
			return Result{}, true, fmt.Errorf("conversation: step %q not found in flow %q", stepName, flowName)
		}
		next := cloneData(data)
		delete(next, retryCountKey)
		if err := e.store.Set(userID, flowName, stepName, next); err != nil {
			return Result{}, true, err
		}
		return Result{Prompt: step.Prompt(next), FlowName: flowName}, true, nil
	}
	if input.CallbackData == resumeCancel {
		if err := e.store.Clear(userID); err != nil {
			return Result{}, true, err
		}
		return Result{Finished: true, FlowName: flowName, Data: Data{ResumeCancelledKey: "true"}}, true, nil
	}

	if time.Since(updatedAt) > idleThreshold {
		return e.resumeGateResult(flowName), true, nil
	}

	f, ok := e.flows[flowName]
	if !ok {
		return Result{}, true, fmt.Errorf("conversation: flow %q is not registered", flowName)
	}
	step, ok := f.step(stepName)
	if !ok {
		return Result{}, true, fmt.Errorf("conversation: step %q not found in flow %q", stepName, flowName)
	}

	transition := step.Process(input, data)

	switch transition.kind {
	case outcomeRetry:
		count := retryCount(data) + 1
		next := cloneData(data)
		next[retryCountKey] = count
		if err := e.store.Set(userID, flowName, stepName, next); err != nil {
			return Result{}, true, err
		}
		if count >= 2 {
			return e.resumeGateResult(flowName), true, nil
		}
		prompt := step.Prompt(next)
		prompt.Text = transition.message + "\n\n" + prompt.Text
		return Result{Prompt: prompt, FlowName: flowName}, true, nil

	case outcomeComplete:
		if err := e.store.Clear(userID); err != nil {
			return Result{}, true, err
		}
		return Result{Finished: true, FlowName: flowName, Data: transition.data}, true, nil

	default:
		delete(transition.data, retryCountKey)
		resolved, err := f.advanceThroughSkips(transition.nextStep, transition.data)
		if err != nil {
			return Result{}, true, err
		}
		if resolved == "" {
			if err := e.store.Clear(userID); err != nil {
				return Result{}, true, err
			}
			return Result{Finished: true, FlowName: flowName, Data: transition.data}, true, nil
		}
		nextStep, ok := f.step(resolved)
		if !ok {
			return Result{}, true, fmt.Errorf("conversation: step %q not found in flow %q", resolved, flowName)
		}
		if err := e.store.Set(userID, flowName, resolved, transition.data); err != nil {
			return Result{}, true, err
		}
		return Result{Prompt: nextStep.Prompt(transition.data), FlowName: flowName}, true, nil
	}
}

func (e *Engine) resumeGateResult(flowName string) Result {
	label := e.resumeLabel(flowName)
	return Result{
		Prompt: Prompt{
			Text: "Che, veo que quedamos a mitad de " + label + " — ¿retomamos o cancelamos?",
			Buttons: []Button{
				{Label: "🔄 Retomar", Data: resumeContinue},
				{Label: "🚫 Cancelar", Data: resumeCancel},
			},
		},
		FlowName: flowName,
	}
}

func cloneData(data Data) Data {
	if data == nil {
		return Data{}
	}
	return maps.Clone(data)
}

func retryCount(data Data) int {
	switch v := data[retryCountKey].(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}
