package messaging

import (
	"context"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

const (
	nudgeCorrectTip    = "correct_tip"
	nudgeQueryTip      = "query_tip"
	nudgeReminderOffer = "reminder_offer"
	nudgeTransferTip   = "transfer_tip"

	nudgeBalanceTip     = "query_balance_tip"
	nudgeTopCategoryTip = "query_top_category_tip"
	nudgeRecentTip      = "query_recent_tip"
	nudgeEnoughTip      = "query_enough_tip"
	nudgePaceTip        = "query_pace_tip"
	nudgeCompareTip     = "query_compare_tip"
	nudgeUsdHoldingsTip = "query_usd_holdings_tip"

	// nudgeQueryPrefix marca el callback del botón de un tip. Se rutea en
	// handleConversationInput ANTES del engine, así un flow abierto no se come
	// el tap como si fuera una opción suya.
	nudgeQueryPrefix = "nudge_q:"

	// Dos cooldowns. Los reactivos responden a algo que el usuario acaba de
	// hacer y pueden salir a diario; los de pregunta son ocho, y a 20h serían
	// ocho días seguidos de tips, que se lee como que el bot no te deja en paz.
	nudgeCooldown         = 20 * time.Hour // ~1 por día
	questionNudgeCooldown = 44 * time.Hour // ~1 cada dos días

	queryTipMin    = 5 // movimientos en los últimos 7 días
	transferTipMin = 2 // cuentas
	reminderDays   = 3
)

type nudgeDef struct {
	key string
	// when recibe la snapshot de actividad compartida: los gates de densidad
	// se resuelven sobre ella, sin una query por gate.
	when func(c *controller, userID uint64, s *nudgeStats) bool
	text string
	// question, si no está vacía, cuelga un botón del tip con esta pregunta
	// como label. El label ES la pregunta a propósito: el usuario aprende la
	// frase y después la puede escribir solo.
	question string
}

// cooldown: los tips de pregunta salen más espaciados que los reactivos.
func (n nudgeDef) cooldown() time.Duration {
	if n.question != "" {
		return questionNudgeCooldown
	}
	return nudgeCooldown
}

// nudgeQuestion devuelve la pregunta de una key, o "" si no existe o no tiene
// pregunta. Es la validación del callback: una key desconocida no dispara nada.
func nudgeQuestion(key string) string {
	for _, n := range nudges {
		if n.key == key {
			return n.question
		}
	}
	return ""
}

var nudges = []nudgeDef{
	{
		key:  nudgeCorrectTip,
		when: func(c *controller, userID uint64, s *nudgeStats) bool { return s.total >= 1 },
		text: "💡 Tip: si te equivocaste, decime «el súper eran 600» y lo corrijo.",
	},
	{
		key: nudgeReminderOffer,
		when: func(c *controller, userID uint64, s *nudgeStats) bool {
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
		when: func(c *controller, userID uint64, s *nudgeStats) bool { return c.hasMultipleAccountsNoTransfer(userID) },
		text: "🔄 Tip: movés plata entre tus cuentas así: «pasé 50 mil del banco a MP».",
	},

	// Los tips de pregunta, ordenados por VALOR decreciente y no por umbral:
	// gana el primero que matchea, así que el orden es la prioridad. El
	// criterio de admisión no es "¿se puede contestar?" sino "¿la respuesta
	// cambia algo?" — un número que el usuario ya podía adivinar no vale un
	// mensaje, y un tip flojo entrena a ignorar el 💡.
	{
		key: nudgeBalanceTip,
		when: func(c *controller, userID uint64, s *nudgeStats) bool {
			return hasActivityFloor(s) &&
				s.total >= balanceTipMovs &&
				c.countAccounts(userID) >= balanceTipAccounts
		},
		text:     "💡 Llevo el saldo de cada cuenta al día, sin que hagas nada.",
		question: "¿Cuánto tengo en cada cuenta?",
	},
	{
		key: nudgeTopCategoryTip,
		when: func(c *controller, userID uint64, s *nudgeStats) bool {
			return hasActivityFloor(s) &&
				s.movsInMonth(0) >= topCategoryTipMovs &&
				c.distinctCategoriesThisMonth(userID) >= topCategoryTipCats
		},
		text:     "💡 ¿Sabías? Puedo decirte en qué se te va la plata.",
		question: "¿En qué gasté más este mes?",
	},
	{
		key: nudgeQueryTip,
		when: func(c *controller, userID uint64, s *nudgeStats) bool {
			return hasActivityFloor(s) && s.movsSince(activityFloorDays) >= queryTipMin
		},
		text:     "💡 ¿Sabías? No hace falta que saques la cuenta vos. Tocá y te digo:",
		question: "¿Cuánto gasté esta semana?",
	},
	{
		key: nudgeRecentTip,
		when: func(c *controller, userID uint64, s *nudgeStats) bool {
			return hasActivityFloor(s) && s.movsSince(activityFloorDays) >= recentTipMovs
		},
		text:     "💡 Che, ¿querés repasar lo último que cargaste?",
		question: "Mostrame mis últimos gastos",
	},
	{
		key: nudgeEnoughTip,
		when: func(c *controller, userID uint64, s *nudgeStats) bool {
			return hasActivityFloor(s) &&
				dayOfMonth() >= enoughTipMinDay &&
				c.hasEnoughDataForMonthVerdict(userID, s)
		},
		text:     "💡 Ya estamos cerca de fin de mes. ¿Sacamos la cuenta?",
		question: "¿Me alcanzó lo que entró este mes?",
	},
	{
		key: nudgePaceTip,
		when: func(c *controller, userID uint64, s *nudgeStats) bool {
			d := dayOfMonth()
			return hasActivityFloor(s) &&
				d >= paceTipMinDay && d <= paceTipMaxDay &&
				s.movsInMonth(0) >= paceTipMovs &&
				s.activeDaysSince(activityFloorDays) >= paceTipActiveDays
		},
		text:     "💡 Con lo que va del mes ya puedo estimarte cómo termina:",
		question: "A este ritmo, ¿cuánto voy a gastar este mes?",
	},
	{
		key: nudgeCompareTip,
		when: func(c *controller, userID uint64, s *nudgeStats) bool {
			return hasActivityFloor(s) &&
				dayOfMonth() >= compareTipMinDay &&
				s.movsInMonth(0) >= compareTipMonthMovs &&
				s.movsInMonth(1) >= compareTipMonthMovs
		},
		text:     "💡 Ya tenés dos meses cargados. Se pueden comparar:",
		question: "¿Gasté más que el mes pasado?",
	},
	{
		key: nudgeUsdHoldingsTip,
		when: func(c *controller, userID uint64, s *nudgeStats) bool {
			return hasActivityFloor(s) && c.hasUsdHoldings(userID)
		},
		text:     "💡 Tus dólares van por separado de los pesos, siempre.",
		question: "¿Cuántos dólares tengo?",
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
// nunca con un flow en curso; cooldown por tipo de tip; once-ever por
// (user, nudge). Best-effort: cualquier error se loguea y se sigue.
func (c *controller) maybeNudge(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) {
	if c.nudges == nil || b == nil {
		return
	}
	if inProgress, err := c.engine.InProgress(userID); err != nil || inProgress {
		return
	}
	keys, err := c.nudges.SentKeys(userID)
	if err != nil {
		return
	}
	sent := make(map[string]bool, len(keys))
	for _, k := range keys {
		sent[k] = true
	}
	last, err := c.nudges.LastSentAt(userID)
	if err != nil {
		return
	}
	// Corte barato: nudgeCooldown es el más chico de todos, así que dentro de
	// esa ventana no puede salir ningún tip y se vuelve sin armar la snapshot.
	// Los cooldowns por tipo filtran después, dentro del loop.
	if last != nil && time.Since(*last) < nudgeCooldown {
		return
	}

	stats := c.buildNudgeStats(userID)
	stats.sent = sent

	for _, n := range nudges {
		if sent[n.key] {
			continue
		}
		if last != nil && time.Since(*last) < n.cooldown() {
			continue
		}
		if !n.when(c, userID, stats) {
			continue
		}
		if err := c.nudges.MarkSent(userID, n.key); err != nil {
			slog.ErrorContext(ctx, "nudge mark failed", "key", n.key, "err", err)
			return
		}
		if n.question != "" {
			c.sendPrompt(ctx, b, chatID, conversation.Prompt{
				Text:    n.text,
				Buttons: []conversation.Button{{Label: n.question, Data: nudgeQueryPrefix + n.key}},
			})
		} else {
			c.sendText(ctx, b, chatID, n.text)
		}
		return
	}
}
