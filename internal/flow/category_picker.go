package flow

import "lopiibot.com/internal/conversation"

// CategoryOptions arma un botón por categoría disponible para el usuario
// (propias + globales, sin reservadas — eso lo filtra DistinctCategoriesForUser),
// todos apuntando a nextStep, y después agrega las opciones extra tal cual.
//
// Las extras van por parámetro y no hardcodeadas porque cada flujo tiene un
// grafo distinto: el wizard suma "crear categoría nueva" apuntando a un step
// suyo, y category_manage_target no tiene ese step. Meterlo fijo acá haría que
// NewFlow falle al validar el grafo del segundo flujo.
//
// Un error del lister devuelve solo las extras: el usuario se queda sin
// categorías para elegir, pero nunca sin botón para salir.
func CategoryOptions(subs categoryLister, data conversation.Data, nextStep string, extra ...conversation.ChoiceOption) []conversation.ChoiceOption {
	cats, _ := subs.DistinctCategoriesForUser(data.UserID())
	opts := make([]conversation.ChoiceOption, 0, len(cats)+len(extra))
	for _, cat := range cats {
		opts = append(opts, conversation.ChoiceOption{
			Label:    subs.IconForCategory(data.UserID(), cat) + " " + cat,
			Value:    cat,
			NextStep: nextStep,
		})
	}
	return append(opts, extra...)
}

// BackOptionTo es el botón Atrás estándar de los flujos multi-step.
func BackOptionTo(step string) conversation.ChoiceOption {
	return conversation.ChoiceOption{Label: "⬅️ Atrás", Value: OptionBack, NextStep: step}
}
