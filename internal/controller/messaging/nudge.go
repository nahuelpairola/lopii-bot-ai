package messaging

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/subcategory"
)

const (
	// nudgeCorrectTip vive en flow (NudgeCorrectTip) — la marca el finish de
	// CREATE; el alias conserva el nombre corto para la lista de nudges.
	nudgeCorrectTip    = flow.NudgeCorrectTip
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
	// nudgeMenuData es el callback del botón "Preguntame" del tip recurrente.
	nudgeMenuData = "nudge_menu"
	nudgeMenuTip  = "query_menu_tip"

	// Dos cooldowns. Los reactivos responden a algo que el usuario acaba de
	// hacer y pueden salir a diario; los de pregunta son ocho, y a 20h serían
	// ocho días seguidos de tips, que se lee como que el bot no te deja en paz.
	nudgeCooldown         = 20 * time.Hour // ~1 por día
	questionNudgeCooldown = 44 * time.Hour // ~1 cada dos días
	// menuNudgeCooldown: el recurrente sale cada ~7 días, no cada dos. Es un
	// recordatorio, no una lección.
	menuNudgeCooldown = 7 * 24 * time.Hour

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
	// recurring: solo el menú. Cambia el cooldown, cómo se marca
	// (MarkSentAgain) y hace que el once-ever no lo frene.
	recurring bool
}

// cooldown: los tips de pregunta salen más espaciados que los reactivos.
func (n nudgeDef) cooldown() time.Duration {
	switch {
	case n.recurring:
		return menuNudgeCooldown
	case n.question != "":
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

// nudges se llena en init() y no en el literal de la var por una restricción
// del compilador: el gate del menú llama a eligibleQuestions, que a su vez
// recorre nudges. En runtime no hay ciclo (el closure corre mucho después del
// arranque), pero el análisis de inicialización de Go lo sigue a través del
// cuerpo de la función y lo rechaza igual. Moverlo a init() lo evita sin
// partir la lista en dos lugares.
var nudges []nudgeDef

func init() {
	nudges = []nudgeDef{
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

		{
			key: nudgeMenuTip,
			// "Todo lo que hoy te puedo ofrecer, ya te lo ofrecí" — y NO "ya te
			// mandé todos los tips". Hay gates que ciertos usuarios no cumplen
			// nunca (una sola cuenta => nudgeBalanceTip jamás; recordatorio ya
			// configurado => nudgeReminderOffer jamás), así que un contador de
			// pendientes no llegaría a cero y el menú no saldría NUNCA para ellos,
			// que son justo a los que se les acabaron los tips.
			when: func(c *controller, userID uint64, s *nudgeStats) bool {
				if !hasActivityFloor(s) {
					return false
				}
				elig := c.eligibleQuestions(userID, s)
				if len(elig) == 0 {
					return false // todavía no hay nada que ofrecer
				}
				for _, n := range elig {
					if !s.sent[n.key] {
						return false // queda una frase por enseñar
					}
				}
				return true
			},
			text:      "💬 ¿Te saco una cuenta?",
			recurring: true,
		},
	}
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

// handleNudgeQuery atiende el tap del botón de un tip. Devuelve true si el
// callback era suyo (y ya lo respondió), false si no le corresponde.
//
// Corre ANTES del engine a propósito: si el usuario toca un botón viejo con un
// flow abierto, el flow no se come el tap como si fuera una opción suya. La
// consulta es read-only, así que el flow queda intacto esperando su input.
//
// Se saltea el loop: ya sabemos que es QUERY, y evitarlo
// ahorra ~700 tokens por tap. Por eso tampoco escribe en intent_events: esa
// tabla mide qué tan bien clasifica el router, y acá no hubo clasificación que
// evaluar. El tap se mide en user_nudges.tapped_at.
//
// Si Groq está caído el tap se pierde con msgQueryFailed, y está bien: el
// botón sigue tocable en el historial, así que el reintento es tocarlo de
// nuevo. Encolarlo sería contestar veinte minutos tarde algo que ya no importa.
func (c *controller) handleNudgeQuery(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, data string) bool {
	if data == nudgeMenuData {
		c.sendQuestionMenu(ctx, b, chatID, userID)
		return true
	}
	if !strings.HasPrefix(data, nudgeQueryPrefix) {
		return false
	}
	key := strings.TrimPrefix(data, nudgeQueryPrefix)
	question := nudgeQuestion(key)
	if question == "" {
		return false
	}
	if c.nudges != nil {
		// Best-effort: perder la métrica nunca vale perder la respuesta.
		if err := c.nudges.MarkTapped(userID, key); err != nil {
			slog.ErrorContext(ctx, "nudge tap mark failed", "key", key, "err", err)
		}
	}
	// La copy de fracaso la manda el caller (ver handleQuery): acá se manda siempre,
	// incluso con un 429, que es justo lo que dice el comentario de arriba — el tap
	// no se encola porque el botón sigue tocable en el historial.
	if answered, err := c.handleQuery(ctx, b, chatID, userID, question); !answered {
		if err != nil {
			slog.ErrorContext(ctx, "nudge query failed", "key", key, "err", err)
		}
		c.sendText(ctx, b, chatID, msgQueryFailed)
	}
	return true
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
		if sent[n.key] && !n.recurring {
			continue
		}
		if last != nil && time.Since(*last) < n.cooldown() {
			continue
		}
		if !n.when(c, userID, stats) {
			continue
		}
		mark := c.nudges.MarkSent
		if n.recurring {
			mark = c.nudges.MarkSentAgain
		}
		if err := mark(userID, n.key); err != nil {
			slog.ErrorContext(ctx, "nudge mark failed", "key", n.key, "err", err)
			return
		}
		switch {
		case n.recurring:
			c.sendPrompt(ctx, b, chatID, conversation.Prompt{
				Text:    n.text,
				Buttons: []conversation.Button{{Label: msgMenuButton, Data: nudgeMenuData}},
			})
		case n.question != "":
			c.sendPrompt(ctx, b, chatID, conversation.Prompt{
				Text:    n.text,
				Buttons: []conversation.Button{{Label: n.question, Data: nudgeQueryPrefix + n.key}},
			})
		default:
			c.sendText(ctx, b, chatID, n.text)
		}
		return
	}
}
