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
