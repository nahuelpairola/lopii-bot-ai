package messaging

import (
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
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
// position that slipped through as text).
func onboardingRowsFromDrafts(drafts []orchestrator.OnboardingAccountDraft) []onboardingRow {
	rows := make([]onboardingRow, 0, len(drafts))
	for _, d := range drafts {
		if _, err := decimal.NewFromString(d.Balance); err != nil {
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
	OnboardingCollectFlowName = onboardingCollectFlowName
	onboardingConfirmFlowName = "onboarding_confirm"

	stepOnboardingAsk     = "onboarding_ask_distribution"
	stepOnboardingDone    = "onboarding_collect_done"
	stepOnboardingConfirm = "onboarding_confirm_accounts"
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
// (Editar is added in Task 9.)
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
