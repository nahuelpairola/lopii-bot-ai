package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/agent"
)

// sendText is a small helper that guards every b.SendMessage call with a
// nil check — b is nil in unit tests that exercise these entry points
// directly (see free_text_test.go), matching the same guard pattern
// already used throughout movement_update_flow.go/movement_delete_flow.go.
func (c *controller) sendText(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	if b == nil {
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
}

// handleFreeText es EL punto de entrada de cualquier mensaje sin flow abierto.
//
// Ya no hay router: el loop unificado lee el mensaje y elige una de sus 7
// herramientas. El switch de diez ramas que había acá repartía el mensaje a
// diez destinos distintos, y esa era la falla: el 2026-08-10 un usuario intentó
// UNA cosa —mover los movimientos del lote a otra categoría— seis veces, y el
// router mandó cada fraseo a un destino distinto donde no había cómo hacerlo.
//
// Los wizards NO se van: siguen siendo la UI del lado de la app. Lo que cambia
// es cómo se llega — el loop parkea en ellos, en vez de un router que decide de
// antemano.
func (c *controller) handleFreeText(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	return agent.StartLoop(ctx, c, b, chatID, userID, text)
}
