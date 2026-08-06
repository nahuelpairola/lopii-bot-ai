package messaging

import (
	"context"
	"math/rand"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
)

// menuMaxOptions: cuántas preguntas se muestran como mucho. Un menú de ocho
// botones es una lista de precios; cuatro se leen de un vistazo.
const menuMaxOptions = 4

// eligibleQuestions devuelve los tips de pregunta cuyo gate pasa AHORA. El
// menú se arma con los mismos gates que gobiernan los tips, así que nunca
// ofrece algo que va a contestar vacío.
func (c *controller) eligibleQuestions(userID uint64, s *nudgeStats) []nudgeDef {
	var out []nudgeDef
	for _, n := range nudges {
		if n.question == "" || n.recurring {
			continue
		}
		if n.when(c, userID, s) {
			out = append(out, n)
		}
	}
	return out
}

// sendQuestionMenu responde al tap de "Preguntame" con las preguntas que hoy
// califican, barajadas y recortadas, para que no parezca un cartel estático.
func (c *controller) sendQuestionMenu(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) {
	opts := c.eligibleQuestions(userID, c.buildNudgeStats(userID))
	if len(opts) == 0 {
		c.sendText(ctx, b, chatID, msgMenuNoData)
		return
	}
	rand.Shuffle(len(opts), func(i, j int) { opts[i], opts[j] = opts[j], opts[i] })
	if len(opts) > menuMaxOptions {
		opts = opts[:menuMaxOptions]
	}
	buttons := make([]conversation.Button, 0, len(opts))
	for _, n := range opts {
		buttons = append(buttons, conversation.Button{Label: n.question, Data: nudgeQueryPrefix + n.key})
	}
	c.sendPrompt(ctx, b, chatID, conversation.Prompt{Text: msgMenuHeader, Buttons: buttons})
}
