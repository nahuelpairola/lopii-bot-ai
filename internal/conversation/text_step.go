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
}

func (s TextStep) Prompt(data Data) Prompt {
	return Prompt{Text: s.PromptText(data)}
}

func (s TextStep) Process(input Input, data Data) Transition {
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
	return []string{s.NextStep}
}
