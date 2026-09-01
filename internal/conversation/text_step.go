package conversation

import "strings"

type TextStep struct {
	PromptText        func(data Data) string
	DataKey           string
	Validate          func(text string, data Data) (errMsg string)
	NextStep          string
	EscapeOptions     []ChoiceOption
	OnEscape          func(value string, data Data) Data
	OnText            func(text string, data Data) Data
	EscapeOptionsFunc func(data Data) []ChoiceOption
	SkipIf            func(data Data) (nextStep string, ok bool)
}

func (s TextStep) escapeOptions(data Data) []ChoiceOption {
	if s.EscapeOptionsFunc == nil {
		return s.EscapeOptions
	}
	opts := make([]ChoiceOption, 0, len(s.EscapeOptions)+1)
	opts = append(opts, s.EscapeOptions...)
	return append(opts, s.EscapeOptionsFunc(data)...)
}

func (s TextStep) Prompt(data Data) Prompt {
	var buttons []Button
	for _, opt := range s.escapeOptions(data) {
		buttons = append(buttons, Button{Label: opt.Label, Data: opt.Value})
	}
	return Prompt{Text: s.PromptText(data), Buttons: buttons}
}

func (s TextStep) Process(input Input, data Data) Transition {
	for _, opt := range s.escapeOptions(data) {
		if input.CallbackData != opt.Value {
			continue
		}
		next := data
		if s.OnEscape != nil {
			next = s.OnEscape(opt.Value, data)
		}
		if opt.Finish {
			return Complete(next)
		}
		return Advance(opt.NextStep, next)
	}

	text := strings.TrimSpace(input.Text)

	if s.Validate != nil {
		if errMsg := s.Validate(text, data); errMsg != "" {
			return Retry(errMsg)
		}
	}

	next := Data{}
	for k, v := range data {
		next[k] = v
	}
	next[s.DataKey] = text
	if s.OnText != nil {
		next = s.OnText(text, next)
	}

	return Advance(s.NextStep, next)
}

func (s TextStep) PossibleNextSteps() []string {
	steps := []string{s.NextStep}
	for _, opt := range s.EscapeOptions {
		if !opt.Finish {
			steps = append(steps, opt.NextStep)
		}
	}
	return steps
}

func (s TextStep) Skip(data Data) (string, bool) {
	if s.SkipIf == nil {
		return "", false
	}
	return s.SkipIf(data)
}
