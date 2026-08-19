package nudges

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
)

// menuMaxOptions: cuántas preguntas se muestran como mucho. Un menú de ocho
// botones es una lista de precios; cuatro se leen de un vistazo.
const menuMaxOptions = 4

// eligibleQuestions devuelve los tips de pregunta cuyo gate pasa AHORA. El
// menú se arma con los mismos gates que gobiernan los tips, así que nunca
// ofrece algo que va a contestar vacío.
func eligibleQuestions(s Services, userID uint64, stats *nudgeStats) []nudgeDef {
	var out []nudgeDef
	for _, n := range nudges {
		if n.question == "" || n.recurring {
			continue
		}
		if n.when(s, userID, stats) {
			out = append(out, n)
		}
	}
	return out
}

// sendQuestionMenu responde al tap de "Preguntame" con las preguntas que hoy
// califican, en orden de declaración —que ya es orden de valor— y recortadas.
//
// Antes barajaba y DESPUÉS recortaba a cuatro, así que podía tirar las mejores y
// dejar las peores. Y en un menú la predecibilidad vale más que la novedad: los
// botones que se mueven de lugar rompen la memoria muscular. El menú ES un
// índice estable; parecer estático es lo que se busca, no lo que se evita.
func sendQuestionMenu(ctx context.Context, s Services, b *bot.Bot, chatID int64, userID uint64) {
	opts := eligibleQuestions(s, userID, buildNudgeStats(s, userID))
	if len(opts) == 0 {
		s.SendText(ctx, b, chatID, msgMenuNoData)
		return
	}
	if len(opts) > menuMaxOptions {
		opts = opts[:menuMaxOptions]
	}
	buttons := make([]conversation.Button, 0, len(opts))
	for _, n := range opts {
		buttons = append(buttons, conversation.Button{Label: n.question, Data: nudgeQueryPrefix + n.key})
	}
	s.SendPrompt(ctx, b, chatID, conversation.Prompt{Text: msgMenuHeader, Buttons: buttons})
}
