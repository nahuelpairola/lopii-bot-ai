package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/messenger"
)

// sendText manda texto plano por el chat ya resuelto — vía messenger.SendText,
// no por un (bot, chatID) desenvuelto a mano. edgeChat.Send trae su propia
// guarda de bot nil (los tests directos de handleFreeText pasan b=nil), y
// cualquier otro messenger.Chat (FakeChat, uno resuelto por chatResolver) ya
// sabe manejarse solo. Task 8 fix round 1: antes de esto pasaba por
// pairOrLog, que sólo reconocía un edgeChat — un chat resuelto por el drenaje
// de 429 lo hacía fallar en silencio.
func (c *controller) sendText(ctx context.Context, chat messenger.Chat, text string) {
	_ = messenger.SendText(ctx, chat, text)
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
	return agent.StartLoop(ctx, c, newEdgeChat(b, chatID), userID, text)
}
