package conversation

import "fmt"

// Input es lo que el usuario mandó, ya traducido a algo neutro por el
// adaptador de turno (Telegram, web, etc). Solo uno de los dos campos
// viene seteado por evento.
type Input struct {
	Text         string // mensaje de texto libre
	CallbackData string // dato de un botón presionado
}

// Data es el estado acumulado de un flujo a lo largo de sus pasos. Cada
// Step lee y escribe lo que necesita ahí; el motor solo lo persiste.
type Data map[string]any

// Prompt es lo que se le muestra al usuario al entrar a un Step: texto y,
// opcionalmente, botones. No sabe nada de Telegram ni de ningún canal
// concreto — el adaptador lo traduce a su formato real.
type Prompt struct {
	Text    string
	Buttons []Button
}

// Button es un botón genérico: texto visible + dato que vuelve como
// CallbackData cuando se lo presiona.
type Button struct {
	Label string
	Data  string
}

// outcomeKind identifica qué tipo de resultado produjo un Step al
// procesar un input.
type outcomeKind int

const (
	outcomeAdvance outcomeKind = iota
	outcomeRetry
	outcomeComplete
)

// Transition es el resultado de que un Step procese un Input. Se
// construye con los helpers Advance, Retry o Complete — no directamente.
type Transition struct {
	kind     outcomeKind
	nextStep string
	data     Data
	message  string // mensaje de error, solo aplica a Retry
}

// Advance mueve el flujo al Step llamado nextStep, con los datos
// acumulados actualizados.
func Advance(nextStep string, data Data) Transition {
	return Transition{kind: outcomeAdvance, nextStep: nextStep, data: data}
}

// Retry se queda en el mismo Step y le muestra un mensaje de error al
// usuario antes de volver a pedirle el input.
func Retry(message string) Transition {
	return Transition{kind: outcomeRetry, message: message}
}

// Complete termina el flujo. data trae el resultado final acumulado,
// listo para que quien definió el flujo haga algo con él.
func Complete(data Data) Transition {
	return Transition{kind: outcomeComplete, data: data}
}

// Step es un paso de un flujo conversacional.
type Step interface {
	// Prompt arma lo que se le muestra al usuario al entrar a este paso.
	Prompt(data Data) Prompt

	// Process recibe el input del usuario y decide la transición.
	Process(input Input, data Data) Transition

	// PossibleNextSteps declara estáticamente a qué otros pasos podría
	// saltar este Step. Se usa solo para validar el grafo al registrar
	// el Flow, no participa en la decisión real en runtime.
	PossibleNextSteps() []string

	// Skip reports whether this step's data is already satisfied and, if
	// so, where the walk should continue (see Flow.advanceThroughSkips).
	// ok=false means "stop here, show this step's Prompt." ok=true with
	// nextStep=="" means "the flow is complete, nothing left to ask."
	Skip(data Data) (nextStep string, ok bool)
}

// Flow es un conjunto de Steps identificados por nombre, con un punto de
// entrada. No sabe nada de Telegram, HTTP, ni de cómo se persiste el
// progreso — de eso se encarga el motor (Engine).
type Flow struct {
	Name        string
	InitialStep string
	steps       map[string]Step
}

// NewFlow arma un Flow y valida que el grafo de Steps sea consistente:
// el paso inicial existe, y todo paso al que algún Step declara que
// podría saltar también existe dentro del mismo Flow.
func NewFlow(name, initialStep string, steps map[string]Step) (*Flow, error) {
	if _, ok := steps[initialStep]; !ok {
		return nil, fmt.Errorf("flow %q: initial step %q is not defined", name, initialStep)
	}

	for stepName, step := range steps {
		for _, next := range step.PossibleNextSteps() {
			if _, ok := steps[next]; !ok {
				return nil, fmt.Errorf("flow %q: step %q references undefined step %q", name, stepName, next)
			}
		}
	}

	return &Flow{Name: name, InitialStep: initialStep, steps: steps}, nil
}

func (f *Flow) step(name string) (Step, bool) {
	s, ok := f.steps[name]
	return s, ok
}

// advanceThroughSkips walks forward from `from`, following each step's
// Skip(data) as long as it reports ok=true, until it lands on a step
// that returns ok=false (that step's Prompt should be shown) or a step
// signals completion by returning ok=true with an empty nextStep (the
// flow is done, resolved=="" and err==nil). Guards against a
// misconfigured Skip loop with a hop-count ceiling.
func (f *Flow) advanceThroughSkips(from string, data Data) (string, error) {
	current := from
	for hops := 0; hops <= len(f.steps); hops++ {
		step, ok := f.step(current)
		if !ok {
			return "", fmt.Errorf("conversation: step %q not found in flow %q", current, f.Name)
		}
		next, skip := step.Skip(data)
		if !skip {
			return current, nil
		}
		if next == "" {
			return "", nil
		}
		current = next
	}
	return "", fmt.Errorf("conversation: flow %q: Skip loop exceeded %d hops, possible misconfiguration", f.Name, len(f.steps))
}

// UserID lee el userID guardado por el motor al arrancar el flujo (ver
// UserIDKey). Contempla que, tras un ciclo de serialización JSON, los
// números quedan como float64 en vez de uint64.
func (d Data) UserID() uint64 {
	switch v := d[UserIDKey].(type) {
	case uint64:
		return v
	case float64:
		return uint64(v)
	default:
		return 0
	}
}

// pendingErrorKey es una key reservada para que un Step deje un mensaje
// de error visible en el próximo Prompt, incluso si la transición es un
// Advance hacia otro Step (Retry solo permite repetir el Step actual;
// esto cubre el caso de "el error ocurrió acá, pero hay que volver a un
// paso anterior para corregirlo").
const pendingErrorKey = "_pending_error"

// WithPendingError devuelve una copia de Data con un mensaje de error
// que el próximo Prompt va a mostrar (ver PrependPendingError).
func WithPendingError(data Data, message string) Data {
	next := Data{}
	for k, v := range data {
		next[k] = v
	}
	next[pendingErrorKey] = message
	return next
}

// PrependPendingError antepone al texto el mensaje de error dejado por
// WithPendingError, si había alguno, y lo limpia de paso. Los Steps que
// quieran soportar este mecanismo llaman a esto al construir su Prompt;
// es opcional, no rompe a los que no lo usan.
func PrependPendingError(data Data, text string) string {
	msg, ok := data[pendingErrorKey].(string)
	if !ok || msg == "" {
		return text
	}
	return msg + "\n\n" + text
}
