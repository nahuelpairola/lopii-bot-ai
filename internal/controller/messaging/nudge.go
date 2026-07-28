package messaging

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/subcategory"
)

const (
	nudgeCorrectTip    = "correct_tip"
	nudgeQueryTip      = "query_tip"
	nudgeReminderOffer = "reminder_offer"
	nudgeTransferTip   = "transfer_tip"

	nudgeCooldown  = 20 * time.Hour // ~1 por día
	queryTipMin    = 5              // movimientos
	transferTipMin = 2              // cuentas
	reminderDays   = 3
)

type nudgeDef struct {
	key  string
	when func(c *controller, userID uint64) bool
	text string
}

var nudges = []nudgeDef{
	{
		key:  nudgeCorrectTip,
		when: func(c *controller, userID uint64) bool { n, _ := c.movements.CountForUser(userID); return n >= 1 },
		text: "💡 Tip: si te equivocaste, decime «el súper eran 600» y lo corrijo.",
	},
	{
		key: nudgeQueryTip,
		when: func(c *controller, userID uint64) bool {
			n, _ := c.movements.CountForUser(userID)
			return n >= queryTipMin
		},
		text: "💡 ¿Sabías? Preguntame «¿cuánto gasté esta semana?» y te lo saco al toque.",
	},
	{
		key: nudgeReminderOffer,
		when: func(c *controller, userID uint64) bool {
			if rem, _ := c.reminders.FindByUserID(userID); rem != nil {
				return false
			}
			u, err := c.users.FindByID(userID)
			return err == nil && time.Since(u.CreatedAt) >= reminderDays*24*time.Hour
		},
		text: "⏰ ¿Querés que te recuerde cargar los gastos? Escribí «recordame cargar gastos».",
	},
	{
		key:  nudgeTransferTip,
		when: func(c *controller, userID uint64) bool { return c.hasMultipleAccountsNoTransfer(userID) },
		text: "🔄 Tip: movés plata entre tus cuentas así: «pasé 50 mil del banco a MP».",
	},
}

// hasMultipleAccountsNoTransfer: 2+ cuentas y ningún movimiento con
// subcategoría "Sistema | Transferencia" (excluye compra USD / FCI / ajustes,
// que también son Type=Transfer pero otra subcategoría).
func (c *controller) hasMultipleAccountsNoTransfer(userID uint64) bool {
	accs, err := c.accounts.FindByUserID(userID)
	if err != nil || len(accs) < transferTipMin {
		return false
	}
	sub, err := c.subcategories.FindByCategoryAndSubcategory(userID, subcategory.CategorySystem, "Transferencia")
	if err != nil {
		return false
	}
	n, err := c.movements.CountBySubcategory(userID, uint64(sub.ID))
	exists := n > 0
	return err == nil && !exists
}

// maybeNudge dispara como mucho un nudge tras procesar un mensaje. Guard:
// nunca con un flow en curso; cooldown global 1/día; once-ever por
// (user, nudge). Best-effort: cualquier error se loguea y se sigue.
func (c *controller) maybeNudge(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) {
	if c.nudges == nil || b == nil {
		return
	}
	if inProgress, err := c.engine.InProgress(userID); err != nil || inProgress {
		return
	}
	if last, err := c.nudges.LastSentAt(userID); err == nil && last != nil && time.Since(*last) < nudgeCooldown {
		return
	}
	for _, n := range nudges {
		sent, err := c.nudges.WasSent(userID, n.key)
		if err != nil || sent {
			continue
		}
		if !n.when(c, userID) {
			continue
		}
		if err := c.nudges.MarkSent(userID, n.key); err != nil {
			slog.ErrorContext(ctx, "nudge mark failed", "key", n.key, "err", err)
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: n.text})
		return
	}
}
