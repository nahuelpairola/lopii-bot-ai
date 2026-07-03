package conversation

// ChoiceOption es una opción posible dentro de un ChoiceStep: el botón
// que se muestra y qué pasa si el usuario la elige. Si Finish es true,
// el flujo termina al elegirla (NextStep se ignora); si no, avanza a
// NextStep.
type ChoiceOption struct {
	Label    string
	Value    string // va como CallbackData
	NextStep string
	Finish   bool
}

// ChoiceStep es un Step genérico y reutilizable para "elegí una opción
// entre estos botones" (moneda, sí/no, confirmaciones, categoría
// existente, etc). No necesita implementarse a mano para cada caso — se
// configura por datos.
//
// Las opciones pueden ser estáticas (Options, conocidas de antemano —
// ej. ARS/USD) o dinámicas (OptionsFunc, calculadas en runtime contra
// Data o una consulta a DB — ej. categorías ya creadas por el usuario).
// Si ambas están seteadas, OptionsFunc tiene prioridad. Cuando se usa
// OptionsFunc, DeclaredNextSteps es obligatorio: como las opciones reales
// no existen todavía al validar el Flow (dependen de runtime), hay que
// declarar aparte el catálogo fijo de destinos posibles para que la
// validación del grafo (ver NewFlow) pueda seguir haciéndose al arrancar
// el server.
type ChoiceStep struct {
	// PromptText puede ser fijo o calculado a partir de Data acumulada
	// hasta este punto (por eso es función, no string).
	PromptText func(data Data) string
	Options    []ChoiceOption
	// OptionsFunc, si está seteado, reemplaza a Options: se evalúa cada
	// vez que se arma el Prompt o se procesa un input, así puede
	// reflejar datos que cambian (ej. categorías del usuario en DB).
	OptionsFunc func(data Data) []ChoiceOption
	// DeclaredNextSteps es el catálogo de destinos posibles cuando se
	// usa OptionsFunc. Ignorado si solo se usa Options (ahí se infiere
	// solo). Ver el comentario del struct para el porqué.
	DeclaredNextSteps []string
	// OnChoice permite, además de saltar de paso, transformar Data antes
	// de avanzar (ej: guardar la categoría elegida). Si es nil, Data no
	// se modifica.
	OnChoice func(value string, data Data) Data
	// InvalidChoiceMessage se muestra si llega un input que no matchea
	// ninguna opción conocida (no debería pasar en uso normal vía
	// botones, pero protege ante texto libre inesperado).
	InvalidChoiceMessage string
	// SkipIf, if set, is checked before showing this step's Prompt during
	// a seeded/auto-advancing walk (see Engine.StartWithData). Returning
	// ok=true skips this step; nextStep says where to continue (empty
	// nextStep means the flow is complete). nil means never skip — every
	// ChoiceStep that doesn't set this is unaffected.
	SkipIf func(data Data) (nextStep string, ok bool)
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
