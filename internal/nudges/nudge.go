package nudges

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/subcategory"
)

const (
	correctTip         = flow.NudgeCorrectTip
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

	nudgeQueryPrefix = "nudge_q:"
	nudgeMenuData    = "nudge_menu"
	nudgeMenuTip     = "query_menu_tip"

	nudgeCooldown         = 20 * time.Hour
	questionNudgeCooldown = 44 * time.Hour
	menuNudgeCooldown     = 7 * 24 * time.Hour

	queryTipMin    = 5
	transferTipMin = 2
	reminderDays   = 3
)

type nudgeDef struct {
	key       string
	when      func(t *Tips, userID uint64, stats *nudgeStats) bool
	text      string
	question  string
	recurring bool
}

func (n nudgeDef) cooldown() time.Duration {
	switch {
	case n.recurring:
		return menuNudgeCooldown
	case n.question != "":
		return questionNudgeCooldown
	}
	return nudgeCooldown
}

func nudgeQuestion(key string) string {
	for _, n := range nudges {
		if n.key == key {
			return n.question
		}
	}
	return ""
}

var nudges []nudgeDef

func init() {
	nudges = []nudgeDef{
		{
			key:  correctTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool { return stats.total >= 1 },
			text: "💡 Tip: si te equivocaste, decime «el súper eran 600» y lo corrijo.",
		},
		{
			key: nudgeReminderOffer,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				if rem, _ := t.reminders.FindByUserID(userID); rem != nil {
					return false
				}
				u, err := t.users.FindByID(userID)
				return err == nil && time.Since(u.CreatedAt) >= reminderDays*24*time.Hour
			},
			text: "⏰ ¿Querés que te recuerde cargar los gastos? Escribí «recordame cargar gastos».",
		},
		{
			key: nudgeTransferTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				return t.hasMultipleAccountsNoTransfer(userID)
			},
			text: "🔄 Tip: movés plata entre tus cuentas así: «pasé 50 mil del banco a MP».",
		},

		{
			key: nudgeBalanceTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				return hasActivityFloor(stats) &&
					stats.total >= balanceTipMovs &&
					t.countAccounts(userID) >= balanceTipAccounts
			},
			text:     "💡 Llevo el saldo de cada cuenta al día, sin que hagas nada.",
			question: "¿Cuánto tengo en cada cuenta?",
		},
		{
			key: nudgeTopCategoryTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				return hasActivityFloor(stats) &&
					stats.movsInMonth(0) >= topCategoryTipMovs &&
					t.distinctCategoriesThisMonth(userID) >= topCategoryTipCats
			},
			text:     "💡 ¿Sabías? Puedo decirte en qué se te va la plata.",
			question: "¿En qué gasté más este mes?",
		},
		{
			key: nudgeQueryTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				return hasActivityFloor(stats) && stats.movsSince(activityFloorDays) >= queryTipMin
			},
			text:     "💡 ¿Sabías? No hace falta que saques la cuenta vos. Tocá y te digo:",
			question: "¿Cuánto gasté esta semana?",
		},
		{
			key: nudgeRecentTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				return hasActivityFloor(stats) && stats.movsSince(activityFloorDays) >= recentTipMovs
			},
			text:     "💡 Che, ¿querés repasar lo último que cargaste?",
			question: "Mostrame mis últimos gastos",
		},
		{
			key: nudgeEnoughTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				return hasActivityFloor(stats) &&
					dayOfMonth() >= enoughTipMinDay &&
					t.hasEnoughDataForMonthVerdict(userID, stats)
			},
			text:     "💡 Ya estamos cerca de fin de mes. ¿Sacamos la cuenta?",
			question: "¿Me alcanzó lo que entró este mes?",
		},
		{
			key: nudgePaceTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				d := dayOfMonth()
				return hasActivityFloor(stats) &&
					d >= paceTipMinDay && d <= paceTipMaxDay &&
					stats.movsInMonth(0) >= paceTipMovs &&
					stats.activeDaysSince(activityFloorDays) >= paceTipActiveDays
			},
			text:     "💡 Con lo que va del mes ya puedo estimarte cómo termina:",
			question: "A este ritmo, ¿cuánto voy a gastar este mes?",
		},
		{
			key: nudgeCompareTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				return hasActivityFloor(stats) &&
					dayOfMonth() >= compareTipMinDay &&
					stats.movsInMonth(0) >= compareTipMonthMovs &&
					stats.movsInMonth(1) >= compareTipMonthMovs
			},
			text:     "💡 Ya tenés dos meses cargados. Se pueden comparar:",
			question: "¿Gasté más que el mes pasado?",
		},
		{
			key: nudgeUsdHoldingsTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				return hasActivityFloor(stats) && t.hasUsdHoldings(userID)
			},
			text:     "💡 Tus dólares van por separado de los pesos, siempre.",
			question: "¿Cuántos dólares tengo?",
		},

		{
			key: nudgeMenuTip,
			when: func(t *Tips, userID uint64, stats *nudgeStats) bool {
				if !hasActivityFloor(stats) {
					return false
				}
				elig := t.eligibleQuestions(userID, stats)
				if len(elig) == 0 {
					return false
				}
				for _, n := range elig {
					if !stats.sent[n.key] {
						return false
					}
				}
				return true
			},
			text:      "💬 ¿Te saco una cuenta?",
			recurring: true,
		},
	}
}

func (t *Tips) hasMultipleAccountsNoTransfer(userID uint64) bool {
	accs, err := t.accounts.FindByUserID(userID)
	if err != nil || len(accs) < transferTipMin {
		return false
	}
	sub, err := t.subcategories.FindByCategoryAndSubcategory(userID, subcategory.CategorySystem, subcategory.SubTransfer)
	if err != nil {
		return false
	}
	n, err := t.movements.CountBySubcategory(userID, uint64(sub.ID))
	exists := n > 0
	return err == nil && !exists
}

func (t *Tips) HandleCallback(ctx context.Context, chat messenger.Chat, userID uint64, data string) bool {
	if data == nudgeMenuData {
		t.sendQuestionMenu(ctx, chat, userID)
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
	if t.history != nil {
		if err := t.history.MarkTapped(userID, key); err != nil {
			slog.ErrorContext(ctx, "nudge tap mark failed", "key", key, "err", err)
		}
	}
	if answered, err := t.handleQuery(ctx, chat, userID, question); !answered {
		if err != nil {
			slog.ErrorContext(ctx, "nudge query failed", "key", key, "err", err)
		}
		flow.SendText(ctx, chat, msgQueryFailed)
	}
	return true
}

func (t *Tips) Maybe(ctx context.Context, chat messenger.Chat, userID uint64) {
	if t.history == nil {
		return
	}
	if inProgress, err := t.engine.InProgress(userID); err != nil || inProgress {
		return
	}
	keys, err := t.history.SentKeys(userID)
	if err != nil {
		return
	}
	sent := make(map[string]bool, len(keys))
	for _, k := range keys {
		sent[k] = true
	}
	last, err := t.history.LastSentAt(userID)
	if err != nil {
		return
	}
	if last != nil && time.Since(*last) < nudgeCooldown {
		return
	}

	stats := t.buildNudgeStats(userID)
	stats.sent = sent

	for _, n := range nudges {
		if sent[n.key] && !n.recurring {
			continue
		}
		if last != nil && time.Since(*last) < n.cooldown() {
			continue
		}
		if !n.when(t, userID, stats) {
			continue
		}
		mark := t.history.MarkSent
		if n.recurring {
			mark = t.history.MarkSentAgain
		}
		if err := mark(userID, n.key); err != nil {
			slog.ErrorContext(ctx, "nudge mark failed", "key", n.key, "err", err)
			return
		}
		switch {
		case n.recurring:
			flow.SendPrompt(ctx, chat, conversation.Prompt{
				Text:    n.text,
				Buttons: []conversation.Button{{Label: msgMenuButton, Data: nudgeMenuData}},
			})
		case n.question != "":
			flow.SendPrompt(ctx, chat, conversation.Prompt{
				Text:    n.text,
				Buttons: []conversation.Button{{Label: n.question, Data: nudgeQueryPrefix + n.key}},
			})
		default:
			flow.SendText(ctx, chat, n.text)
		}
		return
	}
}
