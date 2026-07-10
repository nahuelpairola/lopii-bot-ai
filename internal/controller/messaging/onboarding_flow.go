package messaging

import (
	"context"
	"time"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

// onboardingRow is the JSON-safe per-account shape carried inside
// conversation.Data during onboarding — same string-only discipline as
// movementRow (numbers survive the JSONB round-trip only as strings).
type onboardingRow struct {
	Name      string
	Currency  string
	Balance   string
	IsDefault string // "true"/"false" — never a native bool (JSONB round-trip)
}

func encodeOnboardingRows(rows []onboardingRow) []interface{} {
	out := make([]interface{}, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]interface{}{
			"name": r.Name, "currency": r.Currency, "balance": r.Balance, "is_default": r.IsDefault,
		})
	}
	return out
}

func decodeOnboardingRows(data conversation.Data) []onboardingRow {
	raw, _ := data["accounts"].([]interface{})
	rows := make([]onboardingRow, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]interface{})
		rows = append(rows, onboardingRow{
			Name:      stringOrEmpty(m["name"]),
			Currency:  stringOrEmpty(m["currency"]),
			Balance:   stringOrEmpty(m["balance"]),
			IsDefault: stringOrEmpty(m["is_default"]),
		})
	}
	return rows
}

// onboardingRowsFromDrafts applies the same defaults the prompt requests
// (unnamed → "Efectivo", unstated currency → ARS) defensively, and drops any
// draft whose balance doesn't parse as a decimal (e.g. an unwanted asset
// position that slipped through as text) or whose balance is negative.
func onboardingRowsFromDrafts(drafts []orchestrator.OnboardingAccountDraft) []onboardingRow {
	rows := make([]onboardingRow, 0, len(drafts))
	for _, d := range drafts {
		amt, err := decimal.NewFromString(d.Balance)
		if err != nil {
			continue
		}
		if amt.IsNegative() {
			continue
		}
		name := d.Name
		if name == "" {
			name = "Efectivo"
		}
		cur := d.Currency
		if cur == "" {
			cur = currency.ARS.String()
		}
		rows = append(rows, onboardingRow{Name: name, Currency: cur, Balance: d.Balance})
	}
	return rows
}

// markDefaults flags the first account of each currency IsDefault=true (the
// throwaway seed that satisfies the partial unique index — never shown).
func markDefaults(rows []onboardingRow) []onboardingRow {
	seen := map[string]bool{}
	for i := range rows {
		if seen[rows[i].Currency] {
			rows[i].IsDefault = "false"
		} else {
			rows[i].IsDefault = "true"
			seen[rows[i].Currency] = true
		}
	}
	return rows
}

const (
	onboardingCollectFlowName = "onboarding_collect"
	// OnboardingCollectFlowName is the exported name so the admin reset
	// endpoint (controller/admin) can re-fire onboarding without importing
	// unexported symbols.
	OnboardingCollectFlowName       = onboardingCollectFlowName
	onboardingConfirmFlowName       = "onboarding_confirm"
	onboardingReminderOfferFlowName = "onboarding_reminder_offer"

	stepOnboardingAsk           = "onboarding_ask_distribution"
	stepOnboardingDone          = "onboarding_collect_done"
	stepOnboardingConfirm       = "onboarding_confirm_accounts"
	stepOnboardingOfferReminder = "onboarding_offer_reminder"

	offerReminderYes = "reminder_yes"
	offerReminderNo  = "reminder_no"
)

// NewOnboardingCollectFlow is a single free-text step: capture how the user
// describes their money, then complete (the trailing step Skips straight to
// completion — a TextStep can't Complete on the text path, so it Advances to
// a terminal skip step, the same idiom stepResolveAccount uses). The captured
// text is classified by ClassifyOnboarding in finishOnboardingCollectFlow —
// a Step can't make an LLM call, hence the two-flow chain.
func NewOnboardingCollectFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepOnboardingAsk: conversation.TextStep{
			PromptText: func(conversation.Data) string { return msgOnboardingAskDistribution },
			DataKey:    "distribution_text",
			Validate: func(text string, _ conversation.Data) string {
				if text == "" {
					return msgOnboardingNotUnderstood
				}
				return ""
			},
			NextStep: stepOnboardingDone,
		},
		stepOnboardingDone: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return "" }, // never rendered
			SkipIf:     func(conversation.Data) (string, bool) { return "", true },
		},
	}
	flow, err := conversation.NewFlow(onboardingCollectFlowName, stepOnboardingAsk, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// NewOnboardingConfirmFlow shows the parsed accounts and offers Confirmar /
// Reescribir. Started (via StartWithData, seeded with "accounts") only after
// ClassifyOnboarding parsed at least one account. On Confirmar the flow
// completes with no marker → finishOnboardingConfirmFlow inserts; on
// Reescribir it completes with reescribir=true → the collect flow restarts.
// (Editar is added in Task 11.)
func NewOnboardingConfirmFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepOnboardingConfirm: conversation.ChoiceStep{
			PromptText: msgOnboardingConfirm,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: "confirm", Finish: true},
				{Label: "✍️ Reescribir", Value: "rewrite", Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				if value == "rewrite" {
					next["reescribir"] = "true"
				}
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}
	flow, err := conversation.NewFlow(onboardingConfirmFlowName, stepOnboardingConfirm, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// NewOnboardingReminderOfferFlow is a single binary ChoiceStep: after
// onboarding finishes, ask whether to activate the daily expense-logging
// reminder right now. No EscapeOptions/Cancelar — it's a plain Sí/No, not a
// multi-step flow a user needs to bail out of. Chained from
// finishOnboardingConfirmFlow, the same way onboarding_collect chains into
// onboarding_confirm.
func NewOnboardingReminderOfferFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepOnboardingOfferReminder: conversation.ChoiceStep{
			PromptText: func(conversation.Data) string { return msgOfferReminder },
			Options: []conversation.ChoiceOption{
				{Label: "✅ Dale", Value: offerReminderYes, Finish: true},
				{Label: "🙅 Ahora no", Value: offerReminderNo, Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				if value == offerReminderYes {
					next["offer"] = "yes"
				} else {
					next["offer"] = "no"
				}
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}
	flow, err := conversation.NewFlow(onboardingReminderOfferFlowName, stepOnboardingOfferReminder, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// finishOnboardingCollectFlow runs Call 2 on the captured free text. Zero
// parsed accounts → re-prompt (don't show an empty confirmation). Otherwise
// start the confirm flow seeded with the parsed rows, defaults marked.
func (c *controller) finishOnboardingCollectFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	userID := data.UserID()
	result, err := c.orchestrator.ClassifyOnboarding(ctx, stringOrEmpty(data["distribution_text"]))
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	rows := markDefaults(onboardingRowsFromDrafts(result.Accounts))
	if len(rows) == 0 {
		c.sendText(ctx, b, chatID, msgOnboardingNotUnderstood)
		c.startFlowIfNotBusy(ctx, b, chatID, userID, onboardingCollectFlowName)
		return
	}
	seed := conversation.Data{"accounts": encodeOnboardingRows(rows)}
	prompt, err := c.engine.StartWithData(userID, onboardingConfirmFlowName, seed)
	if err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
}

// finishOnboardingConfirmFlow either restarts collection (Reescribir) or does
// the atomic insert + receipt + capabilities showcase (Confirmar).
func (c *controller) finishOnboardingConfirmFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if stringOrEmpty(data["reescribir"]) == "true" {
		c.startFlowIfNotBusy(ctx, b, chatID, data.UserID(), onboardingCollectFlowName)
		return
	}
	rows := decodeOnboardingRows(data)
	if err := c.insertOnboardingAccounts(data.UserID(), rows); err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	c.sendText(ctx, b, chatID, msgOnboardingReceipt(rows))
	c.sendText(ctx, b, chatID, msgCapabilitiesShowcase)
	c.startFlowIfNotBusy(ctx, b, chatID, data.UserID(), onboardingReminderOfferFlowName)
}

// insertOnboardingAccounts builds one AccountOpening per row (opening
// transfer, Sistema | Saldo inicial) and inserts them all in one tx.
func (c *controller) insertOnboardingAccounts(userID uint64, rows []onboardingRow) error {
	sub, err := c.subcategories.FindByCategoryAndSubcategory(userID, "Sistema", "Saldo inicial")
	if err != nil {
		return err
	}
	items := make([]movement.AccountOpening, 0, len(rows))
	for _, row := range rows {
		amount, err := decimal.NewFromString(row.Balance)
		if err != nil {
			return err
		}
		cur := currency.Currency(row.Currency)
		items = append(items, movement.AccountOpening{
			Account: &account.Account{UserID: userID, Name: row.Name, Currency: cur, IsDefault: row.IsDefault == "true"},
			Movement: movement.Movement{
				UserID:        userID,
				SubcategoryID: uint64(sub.ID),
				Date:          time.Now(),
				Type:          movement.Transfer,
				Amount:        amount,
				Currency:      cur,
			},
		})
	}
	return c.movements.InsertAccountsWithOpenings(items)
}

// finishOnboardingReminderOffer applies the onboarding reminder offer: "Sí"
// hands off to the exact same entry point a REMINDER_SET intent would use
// (startReminderSetup) — no separate window-picking logic. "No" is a no-op.
func (c *controller) finishOnboardingReminderOffer(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if stringOrEmpty(data["offer"]) != "yes" {
		return
	}
	c.startReminderSetup(ctx, b, chatID, data.UserID())
}
