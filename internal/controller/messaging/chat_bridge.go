package messaging

import (
	"context"
	"log/slog"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/messenger/telegram"
	"lopiibot.com/internal/nudges"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/settings"
)

// edgeChat es un messenger.Chat que retiene el (bot, chatID) que lo armó.
// PUENTE TEMPORAL de la Task 5: flow y agent ya sólo conocen messenger.Chat,
// pero settings, pendingjob y query (Tasks 7/8) todavía piden el par crudo. Es
// el único lugar del árbol donde ese par se puede recuperar de un
// messenger.Chat opaco — se borra cuando esos tres paquetes migren.
type edgeChat struct {
	messenger.Chat
	b      *bot.Bot
	chatID int64
}

// newEdgeChat es lo que este paquete usa en vez de telegram.ChatFrom en todo
// sitio que arranca una llamada a flow o a agent: el resultado tiene que poder
// volver a dar (bot, chatID) más adelante, en los puentes hacia settings/
// pendingjob/query.
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

// pairOrLog es lo que cada puente de este archivo usa en vez de llamar a
// asTelegramPair a mano: hoy el `false` nunca pasa —todo call site de este
// paquete arranca con newEdgeChat— pero es un invariante que sostiene la
// convención, no el compilador. Si algún día llega acá un messenger.Chat que
// no es un edgeChat, silencio total sería peor que un log: cada
// sendText/sendPrompt/startFlow de más abajo haría un no-op mudo con (nil, 0).
// Muere junto con el resto de chat_bridge.go cuando settings/pendingjob/
// nudges/query migren (Tasks 6-8).
func pairOrLog(ctx context.Context, chat messenger.Chat, method string) (*bot.Bot, int64, bool) {
	b, chatID, ok := asTelegramPair(chat)
	if !ok {
		slog.ErrorContext(ctx, "chat_bridge: el messenger.Chat recibido no es un edgeChat, no se puede recuperar (bot, chatID)", "method", method)
	}
	return b, chatID, ok
}

// settingsBridge implementa settings.Services traduciendo las llamadas viejas
// —(bot, chatID)— a los métodos nuevos de *controller, basados en
// messenger.Chat. PUENTE TEMPORAL: se borra cuando internal/settings migre
// (Task 7). El resto de los métodos de settings.Services los promueve el
// *controller embebido sin cambios.
type settingsBridge struct{ *controller }

var _ settings.Services = settingsBridge{}

func (s settingsBridge) SendText(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	s.controller.SendText(ctx, newEdgeChat(b, chatID), text)
}

func (s settingsBridge) SendPrompt(ctx context.Context, b *bot.Bot, chatID int64, prompt conversation.Prompt) {
	s.controller.SendPrompt(ctx, newEdgeChat(b, chatID), prompt)
}

func (s settingsBridge) StartFlow(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, flowName string, seed conversation.Data, errCtx string) error {
	return s.controller.StartFlow(ctx, newEdgeChat(b, chatID), userID, flowName, seed, errCtx)
}

func (s settingsBridge) HandleGroqError(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string, err error) (bool, error) {
	return s.controller.HandleGroqError(ctx, newEdgeChat(b, chatID), userID, text, err)
}

// pendingjobBridge implementa pendingjob.Services: sólo SendText choca de
// nombre con flow.runner/agentServices, así que es el único método que hace
// falta traducir acá. PUENTE TEMPORAL: se borra cuando internal/pendingjob
// migre (Task 8).
type pendingjobBridge struct{ *controller }

var _ pendingjob.Services = pendingjobBridge{}

func (p pendingjobBridge) SendText(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	p.controller.SendText(ctx, newEdgeChat(b, chatID), text)
}

// PendingJobServices expone pendingjobBridge fuera del paquete: pendingjob.Run
// se llama desde server.go, y *controller (tipo no exportado) ya no satisface
// pendingjob.Services directamente. PUENTE TEMPORAL: se borra junto con
// pendingjobBridge cuando internal/pendingjob migre (Task 8).
func (c *controller) PendingJobServices() pendingjob.Services {
	return pendingjobBridge{c}
}

// queryBridge implementa la interfaz no exportada query.services: sólo
// QuerySendText necesitaba la firma nueva del lado del *controller (pedido
// explícito de la Task 5), así que acá se traduce de vuelta. Al ser
// `services` no exportada no hay un `var _` que la nombre desde afuera — la
// satisface estructuralmente el uso en query_services.go. PUENTE TEMPORAL: se
// borra cuando internal/query migre (Task 7).
type queryBridge struct{ *controller }

func (q queryBridge) QuerySendText(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	q.controller.QuerySendText(ctx, newEdgeChat(b, chatID), text)
}

// nudgesBridge implementa nudges.Services: SendText y SendPrompt chocan de
// firma con flow.runner/agentServices igual que en los otros puentes.
// PUENTE TEMPORAL: se borra cuando internal/nudges migre (Task 7).
type nudgesBridge struct{ *controller }

var _ nudges.Services = nudgesBridge{}

func (n nudgesBridge) SendText(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	n.controller.SendText(ctx, newEdgeChat(b, chatID), text)
}

func (n nudgesBridge) SendPrompt(ctx context.Context, b *bot.Bot, chatID int64, prompt conversation.Prompt) {
	n.controller.SendPrompt(ctx, newEdgeChat(b, chatID), prompt)
}
