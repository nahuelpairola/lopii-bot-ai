package conversation

import (
	"fmt"
	"time"
)

// Result es lo que el motor le devuelve a un adaptador (messaging, web,
// etc) después de procesar un input: qué mostrarle al usuario, y si el
// flujo en cuestión terminó en esta misma interacción.
type Result struct {
	Prompt   Prompt
	Finished bool
	FlowName string // siempre seteado; relevante sobre todo cuando Finished es true
	Data     Data   // solo tiene contenido útil cuando Finished es true
}

// stateStore persiste en qué flujo/paso/datos está cada usuario. La
// implementación real vive en repository.go, contra conversation_states.
type stateStore interface {
	Get(userID uint64) (flowName, stepName string, data Data, updatedAt time.Time, found bool, err error)
	Set(userID uint64, flowName, stepName string, data Data) error
	Clear(userID uint64) error
}

// idleThreshold is how long a user can leave a flow untouched before the
// next interaction shows the resume gate instead of the step's own
// validation error.
const idleThreshold = 10 * time.Minute

// retryCountKey tracks consecutive Retry outcomes on the current step,
// reset to absent on any successful Advance/Complete. A first mismatch
// shows the step's own plain error (avoids interrupting a simple typo); a
// second consecutive one escalates to the resume gate.
const retryCountKey = "_retry_count"

const (
	resumeContinue = "_resume_continue"
	resumeCancel   = "_resume_cancel"
)

// Engine orquesta Flows registrados contra la persistencia de estado.
type Engine struct {
	flows       map[string]*Flow
	store       stateStore
	resumeLabel func(flowName string) string
}

func NewEngine(store stateStore, resumeLabel func(flowName string) string) *Engine {
	return &Engine{flows: make(map[string]*Flow), store: store, resumeLabel: resumeLabel}
}

// Register agrega un Flow ya validado (ver NewFlow) al motor.
func (e *Engine) Register(f *Flow) {
	e.flows[f.Name] = f
}

// UserIDKey es la key reservada bajo la cual el motor guarda el userID
// dentro de Data al arrancar un flujo. Los Steps que necesiten saber
// para qué usuario están corriendo lo leen de ahí, en vez de recibirlo
// por closure — así un mismo Flow (registrado una sola vez) sirve para
// todos los usuarios sin pisarse entre sí.
const UserIDKey = "_user_id"

// Start arranca un flujo desde cero para un usuario y devuelve el primer
// Prompt a mostrar.
func (e *Engine) Start(userID uint64, flowName string) (Prompt, error) {
	return e.StartWithData(userID, flowName, Data{})
}

// StartWithData arranca un flujo pre-cargado con datos ya conocidos
// (por ejemplo, lo que devolvió una clasificación de LLM), saltando
// automáticamente cualquier paso cuya información ya esté resuelta. Si
// el seed no deja nada por preguntar, devuelve un error — quien llama
// no debería arrancar un flujo cuando no hay ningún hueco real.
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

	step, _ := f.step(resolved)
	if err := e.store.Set(userID, f.Name, resolved, data); err != nil {
		return Prompt{}, err
	}
	return step.Prompt(data), nil
}

// InProgress indica si el usuario tiene un flujo activo ahora mismo.
func (e *Engine) InProgress(userID uint64) (bool, error) {
	_, _, _, _, found, err := e.store.Get(userID)
	return found, err
}

// Handle procesa un input para el flujo en curso del usuario. Devuelve
// found=false si el usuario no tiene ningún flujo activo (el adaptador
// decide qué hacer en ese caso, el motor no opina). Antes de despachar al
// Step actual, chequea los sentinels del resume gate y el umbral de
// inactividad — ver resumeGateResult.
func (e *Engine) Handle(userID uint64, input Input) (result Result, found bool, err error) {
	flowName, stepName, data, updatedAt, found, err := e.store.Get(userID)
	if err != nil || !found {
		return Result{}, found, err
	}

	if input.CallbackData == resumeContinue {
		next := cloneData(data)
		delete(next, retryCountKey)
		if err := e.store.Set(userID, flowName, stepName, next); err != nil {
			return Result{}, true, err
		}
		step, _ := e.flows[flowName].step(stepName)
		return Result{Prompt: step.Prompt(next), FlowName: flowName}, true, nil
	}
	if input.CallbackData == resumeCancel {
		if err := e.store.Clear(userID); err != nil {
			return Result{}, true, err
		}
		return Result{Finished: true, FlowName: flowName, Data: Data{"_resume_cancelled": "true"}}, true, nil
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

	default: // outcomeAdvance
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

// resumeGateResult builds the "¿retomamos o cancelamos?" prompt shown by
// both gate triggers (idle timeout, 2nd consecutive Retry). label comes
// from the Engine's resumeLabel resolver, injected at construction time
// — conversation cannot import the messaging package that defines the
// actual per-flow copy (see messaging.FlowResumeLabel).
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

func retryCount(data Data) int {
	switch v := data[retryCountKey].(type) {
	case int:
		return v
	case float64: // post-JSONB-round-trip shape
		return int(v)
	default:
		return 0
	}
}

func cloneData(data Data) Data {
	next := make(Data, len(data)+1)
	for k, v := range data {
		next[k] = v
	}
	return next
}
