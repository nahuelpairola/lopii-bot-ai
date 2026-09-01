package conversation

type ChoiceOption struct {
	Label    string
	Value    string
	NextStep string
	Finish   bool
}

type ChoiceStep struct {
	PromptText           func(data Data) string
	Options              []ChoiceOption
	OptionsFunc          func(data Data) []ChoiceOption
	DeclaredNextSteps    []string
	OnChoice             func(value string, data Data) Data
	InvalidChoiceMessage string
	SkipIf               func(data Data) (nextStep string, ok bool)
}

func (s ChoiceStep) options(data Data) []ChoiceOption {
	if s.OptionsFunc != nil {
		return s.OptionsFunc(data)
	}
	return s.Options
}

func (s ChoiceStep) Prompt(data Data) Prompt {
	var buttons []Button
	for _, opt := range s.options(data) {
		buttons = append(buttons, Button{Label: opt.Label, Data: opt.Value})
	}
	return Prompt{Text: s.PromptText(data), Buttons: buttons}
}

func (s ChoiceStep) Process(input Input, data Data) Transition {
	for _, opt := range s.options(data) {
		if input.CallbackData != opt.Value {
			continue
		}
		next := data
		if s.OnChoice != nil {
			next = s.OnChoice(opt.Value, data)
		}
		if opt.Finish {
			return Complete(next)
		}
		return Advance(opt.NextStep, next)
	}

	msg := s.InvalidChoiceMessage
	if msg == "" {
		msg = "Elegí una de las opciones."
	}
	return Retry(msg)
}

func (s ChoiceStep) PossibleNextSteps() []string {
	if s.OptionsFunc != nil {
		return s.DeclaredNextSteps
	}

	var steps []string
	for _, opt := range s.Options {
		if opt.Finish {
			continue
		}
		steps = append(steps, opt.NextStep)
	}
	return steps
}

func (s ChoiceStep) Skip(data Data) (string, bool) {
	if s.SkipIf == nil {
		return "", false
	}
	return s.SkipIf(data)
}
