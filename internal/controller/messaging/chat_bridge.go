package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/messenger/telegram"
)

// edgeChat es un messenger.Chat que retiene el (bot, chatID) que lo armó, y
// sólo para eso: pisar el nil-bot guard de Send/Typing que este paquete
// necesita en sus propios tests (ver abajo). PUENTE TEMPORAL de la Task 5:
// el borde del webhook todavía arma el chat a mano en cada handler — eso
// migra en la Task 6, que es cuando este tipo se borra.
//
// Task 8, fix round 1: hasta acá edgeChat también existía para que
// asTelegramPair/pairOrLog pudieran desenvolver (bot, chatID) de vuelta, y
// las bridges hacia agent/flow/pendingjob (SendPrompt, StartFlow,
// FinishAnswerQuery, FinishManageSettings, SendText, HandleFreeText) pasaban
// por ahí. Eso rompía en silencio cualquier chat NO armado con newEdgeChat —
// exactamente lo que devuelve chatResolver.ChatFor (el sweeper, el drenaje
// de 429) — porque el type assertion a edgeChat fallaba siempre para un
// chat de otro tipo concreto. Las bridges ahora van directo por
// messenger.SendText/chat.Send; asTelegramPair y pairOrLog quedaron sin
// llamadores y se borraron.
type edgeChat struct {
	messenger.Chat
	b      *bot.Bot
	chatID int64
}

// newEdgeChat es lo que este paquete usa en vez de telegram.ChatFrom en todo
// sitio que arranca una llamada a flow, agent o pendingjob.
func newEdgeChat(b *bot.Bot, chatID int64) edgeChat {
	return edgeChat{Chat: telegram.ChatFrom(b, chatID), b: b, chatID: chatID}
}

// Send y Typing pisan al Chat embebido para conservar, en ESTE paquete
// solamente, la guarda de bot nil que tenían sendText/sendPrompt antes de la
// Task 5 — decenas de tests de controller/messaging arman un controller con
// b == nil y esperan un no-op silencioso, y ese hábito no es de este paquete
// arreglarlo (Task 6). flow y agent, en cambio, NO llevan esta guarda: ahí un
// chat siempre viene de un *messenger.FakeChat o de un bot real.
func (ec edgeChat) Send(ctx context.Context, p conversation.Prompt) error {
	if ec.b == nil {
		return nil
	}
	return ec.Chat.Send(ctx, p)
}

func (ec edgeChat) Typing(ctx context.Context) error {
	if ec.b == nil {
		return nil
	}
	return ec.Chat.Typing(ctx)
}
