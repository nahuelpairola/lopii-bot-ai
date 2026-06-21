package conversation

import "fmt"

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
	Get(userID uint64) (flowName, stepName string, data Data, found bool, err error)
	Set(userID uint64, flowName, stepName string, data Data) error
	Clear(userID uint64) error
}

// Engine orquesta Flows registrados contra la persistencia de estado.
type Engine struct {
	flows map[string]*Flow
	store stateStore
}

func NewEngine(store stateStore) *Engine {
	return &Engine{flows: make(map[string]*Flow), store: store}
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
	f, ok := e.flows[flowName]
	if !ok {
		return Prompt{}, fmt.Errorf("conversation: flow %q is not registered", flowName)
	}

	data := Data{UserIDKey: userID}
	if err := e.store.Set(userID, f.Name, f.InitialStep, data); err != nil {
		return Prompt{}, err
	}

	step, _ := f.step(f.InitialStep)
	return step.Prompt(data), nil
}

// InProgress indica si el usuario tiene un flujo activo ahora mismo.
func (e *Engine) InProgress(userID uint64) (bool, error) {
	_, _, _, found, err := e.store.Get(userID)
	return found, err
}

// Handle procesa un input para el flujo en curso del usuario. Devuelve
// found=false si el usuario no tiene ningún flujo activo (el adaptador
// decide qué hacer en ese caso, el motor no opina).
func (e *Engine) Handle(userID uint64, input Input) (result Result, found bool, err error) {
	flowName, stepName, data, found, err := e.store.Get(userID)
	if err != nil || !found {
		return Result{}, found, err
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
		prompt := step.Prompt(data)
		prompt.Text = transition.message + "\n\n" + prompt.Text
		return Result{Prompt: prompt, FlowName: flowName}, true, nil

	case outcomeComplete:
		if err := e.store.Clear(userID); err != nil {
			return Result{}, true, err
		}
		return Result{Finished: true, FlowName: flowName, Data: transition.data}, true, nil

	default: // outcomeAdvance
		nextStep, ok := f.step(transition.nextStep)
		if !ok {
			return Result{}, true, fmt.Errorf("conversation: step %q not found in flow %q", transition.nextStep, flowName)
		}
		if err := e.store.Set(userID, flowName, transition.nextStep, transition.data); err != nil {
			return Result{}, true, err
		}
		return Result{Prompt: nextStep.Prompt(transition.data), FlowName: flowName}, true, nil
	}
}
