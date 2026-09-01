package conversation

import "fmt"

type Input struct {
	Text         string
	CallbackData string
}

type Data map[string]any

type Prompt struct {
	Text    string
	Buttons []Button
}

type Button struct {
	Label      string
	Data       string
	WebAppPath string
}

type outcomeKind int

const (
	outcomeAdvance outcomeKind = iota
	outcomeRetry
	outcomeComplete
)

type Transition struct {
	kind     outcomeKind
	nextStep string
	data     Data
	message  string
}

func Advance(nextStep string, data Data) Transition {
	return Transition{kind: outcomeAdvance, nextStep: nextStep, data: data}
}

func Retry(message string) Transition {
	return Transition{kind: outcomeRetry, message: message}
}

func Complete(data Data) Transition {
	return Transition{kind: outcomeComplete, data: data}
}

type Step interface {
	Prompt(data Data) Prompt

	Process(input Input, data Data) Transition

	PossibleNextSteps() []string

	Skip(data Data) (nextStep string, ok bool)
}

type Flow struct {
	Name        string
	InitialStep string
	steps       map[string]Step
}

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

const pendingErrorKey = "_pending_error"

func WithPendingError(data Data, message string) Data {
	next := cloneData(data)
	next[pendingErrorKey] = message
	return next
}

func PrependPendingError(data Data, text string) string {
	msg, ok := data[pendingErrorKey].(string)
	if !ok || msg == "" {
		return text
	}
	return msg + "\n\n" + text
}
