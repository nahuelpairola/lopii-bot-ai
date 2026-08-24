package messaging

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/messenger/telegram"
)

// edgeChat es un messenger.Chat que retiene el (bot, chatID) que lo armó.
// PUENTE TEMPORAL de la Task 5: flow, agent y ahora pendingjob (Task 8) ya
// sólo conocen messenger.Chat, pero este paquete todavía necesita el par
// crudo — sendText y sendPrompt (free_text.go, controller.go) no migran
// hasta la Task 6. Es el único lugar del árbol donde ese par se puede
// recuperar de un messenger.Chat opaco, y se borra cuando la Task 6 mude
// sendText/sendPrompt a messenger.Chat.
type edgeChat struct {
	messenger.Chat
	b      *bot.Bot
	chatID int64
}

// newEdgeChat es lo que este paquete usa en vez de telegram.ChatFrom en todo
// sitio que arranca una llamada a flow, agent o pendingjob: el resultado
// tiene que poder volver a dar (bot, chatID) más adelante, vía pairOrLog.
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

// asTelegramPair recupera el (bot, chatID) de un messenger.Chat armado acá. Si
// el chat vino de otro lado (no debería, en este paquete) el segundo valor de
// retorno es false y (nil, 0) no se puede usar.
func asTelegramPair(chat messenger.Chat) (*bot.Bot, int64, bool) {
	ec, ok := chat.(edgeChat)
	if !ok {
		return nil, 0, false
	}
	return ec.b, ec.chatID, true
}

// pairOrLog es lo que este paquete usa (en free_text.go, agent_services.go,
// agent_delegates.go, controller.go) en vez de llamar a asTelegramPair a
// mano: hoy el `false` nunca pasa —todo call site de este paquete arranca
// con newEdgeChat— pero es un invariante que sostiene la convención, no el
// compilador. Si algún día llega acá un messenger.Chat que no es un
// edgeChat, silencio total sería peor que un log: cada
// sendText/sendPrompt/startFlow de más abajo haría un no-op mudo con (nil, 0).
// Muere junto con el resto de chat_bridge.go cuando la Task 6 mude
// sendText/sendPrompt a messenger.Chat.
func pairOrLog(ctx context.Context, chat messenger.Chat, method string) (*bot.Bot, int64, bool) {
	b, chatID, ok := asTelegramPair(chat)
	if !ok {
		slog.ErrorContext(ctx, "chat_bridge: el messenger.Chat recibido no es un edgeChat, no se puede recuperar (bot, chatID)", "method", method)
	}
	return b, chatID, ok
}
