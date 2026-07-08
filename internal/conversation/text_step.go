package conversation

import "strings"

// TextStep es un Step genérico para "pedile texto libre al usuario,
// validalo, y guardalo en Data bajo una key". Reutilizable para nombre
// de cuenta, saldo inicial, o cualquier otro input de texto simple.
type TextStep struct {
	PromptText func(data Data) string
	// DataKey es dónde se guarda el texto validado dentro de Data.
	DataKey string
	// Validate corre antes de aceptar el input. Devuelve un mensaje de
	// error si no es válido, o "" si está OK.
	Validate func(text string, data Data) (errMsg string)
	// NextStep es a dónde se avanza una vez que el input es válido.
	NextStep string
	// EscapeOptions son botones extra junto al prompt de texto libre (ej.
	// Cancelar, Atrás) — se chequean antes de tratar el input como texto.
	// nil = sin botones, comportamiento actual sin cambios.
	EscapeOptions []ChoiceOption
	// OnEscape transforma Data antes de completar/avanzar por una opción
	// de EscapeOptions (ej. marcar cancelación). nil = Data pasa sin
	// cambios — mismo contrato que ChoiceStep.OnChoice.
	OnEscape func(value string, data Data) Data
	// SkipIf, if set, is checked before showing this step's Prompt during
	// a seeded/auto-advancing walk (see Engine.StartWithData). Returning
	// ok=true skips this step; nextStep says where to continue (empty
	// nextStep means the flow is complete). nil means never skip — same
	// contract as ChoiceStep.SkipIf. Used by movement/account/category
	// setup flows to skip over already-resolved steps when starting with
	// pre-seeded data (e.g. an LLM classification).
	SkipIf func(data Data) (nextStep string, ok bool)
}

func (s TextStep) Prompt(data Data) Prompt {
	var buttons []Button
	for _, opt := range s.EscapeOptions {
		buttons = append(buttons, Button{Label: opt.Label, Data: opt.Value})
	}
	return Prompt{Text: s.PromptText(data), Buttons: buttons}
}

func (s TextStep) Process(input Input, data Data) Transition {
	for _, opt := range s.EscapeOptions {
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
