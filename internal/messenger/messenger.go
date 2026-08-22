// Package messenger es la superficie neutra entre lopii y el canal por el que
// habla. Nada fuera de un adapter conoce el tipo concreto de un canal.
//
// La regla: pasar un identificador opaco (Channel) es transparente; ramificar
// sobre él no lo es. No hay ni un `if` sobre Channel fuera de este árbol.
package messenger

import (
	"context"

	"lopiibot.com/internal/conversation"
)

// Chat es una conversación abierta con un usuario en un canal concreto.
// Enlaza transporte y dirección: el par (b *bot.Bot, chatID int64) que viajaba
// por 110 firmas colapsa en este único valor.
type Chat interface {
	Send(ctx context.Context, p conversation.Prompt) error
	Typing(ctx context.Context) error
}

// Incoming es un mensaje entrante ya traducido: quién lo mandó, por dónde se
// le contesta, y qué dijo. Lo produce el adapter.
//
// No lleva ni Kind ni RawText: los dos derivan de Input (un callback trae
// CallbackData, un mensaje trae Text) y se calculan donde se usan, en traced.
type Incoming struct {
	Channel       string
	ChannelUserID string
	DisplayName   string
	Chat          Chat
	Input         conversation.Input
}

// Handler es lo que el borde le da al adapter. Una sola función: el
// onboarding NO pasa por acá — es una convención de deep-link de Telegram y
// vive en su adapter (ver el diseño, § "Onboarding stays outside").
type Handler func(ctx context.Context, in Incoming)

// SendText es un helper de paquete, NO un método de Chat: un Prompt sin
// botones ya es un texto, y un segundo método obligaría a cada implementador
// futuro a escribir lo mismo dos veces.
func SendText(ctx context.Context, c Chat, s string) error {
	return c.Send(ctx, conversation.Prompt{Text: s})
}
