package messaging

import (
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
)

// accountPendingCreate is the sentinel movementRow.AccountID value
// meaning "the user chose, mid-flow, to create this account" — real
// account ids are always numeric strings, so this can never collide.
const accountPendingCreate = "PENDING_CREATE"

// movementRow is the JSON-safe, per-row shape carried inside
// conversation.Data during CREATE's gap-fill flow. Every field is a
// string: conversation.Data round-trips through Postgres JSONB, and
// only strings/bools/slices/maps of those survive that round-trip
// without corruption (numbers decode back as float64 — see
// conversation.Data.UserID's own float64 fallback for why).
type movementRow struct {
	Type             string
	Amount           string
	Currency         string
	AccountID        string
	AccountNameGuess string
	Category         string
	Subcategory      string
	PaymentMethod    string
	Merchant         string
	Description      string
	Date             string
}

func stringOrEmpty(v any) string {
	s, _ := v.(string)
	return s
}

func copyData(data conversation.Data) conversation.Data {
	next := make(conversation.Data, len(data))
	for k, v := range data {
		next[k] = v
	}
	return next
}

func decodeMovementRows(data conversation.Data) []movementRow {
	raw, _ := data["movements"].([]interface{})
	rows := make([]movementRow, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]interface{})
		rows = append(rows, movementRow{
			Type:             stringOrEmpty(m["type"]),
			Amount:           stringOrEmpty(m["amount"]),
			Currency:         stringOrEmpty(m["currency"]),
			AccountID:        stringOrEmpty(m["account_id"]),
			AccountNameGuess: stringOrEmpty(m["account_name_guess"]),
			Category:         stringOrEmpty(m["category"]),
			Subcategory:      stringOrEmpty(m["subcategory"]),
			PaymentMethod:    stringOrEmpty(m["payment_method"]),
			Merchant:         stringOrEmpty(m["merchant"]),
			Description:      stringOrEmpty(m["description"]),
			Date:             stringOrEmpty(m["date"]),
		})
	}
	return rows
}

func encodeMovementRows(rows []movementRow) []interface{} {
	encoded := make([]interface{}, 0, len(rows))
	for _, r := range rows {
		encoded = append(encoded, map[string]interface{}{
			"type":               r.Type,
			"amount":             r.Amount,
			"currency":           r.Currency,
			"account_id":         r.AccountID,
			"account_name_guess": r.AccountNameGuess,
			"category":           r.Category,
			"subcategory":        r.Subcategory,
			"payment_method":     r.PaymentMethod,
			"merchant":           r.Merchant,
			"description":        r.Description,
			"date":               r.Date,
		})
	}
	return encoded
}

func decodeStringSlice(data conversation.Data, key string) []string {
	raw, _ := data[key].([]interface{})
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func encodeStringSlice(items []string) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, s := range items {
		out = append(out, s)
	}
	return out
}

// buildCreateSeed converts a Call 2 CREATE (or a resolved Call 2
// UPDATE) result into the seed Data for the movement_create flow: one
// movementRow per draft, plus a queue of row indices whose
// category/subcategory landed in PENDING_REVIEW and a queue of row
// indices whose transfer referenced an account the LLM couldn't
// resolve to an existing one. mode is "create" or "update";
// oldTransactionID is only meaningful for "update" (see
// movement_update_flow.go builds its own seed (buildUpdateSeed) since an
// UPDATE's shape differs — before/after movements, no gap-filling in
// this feature's scope — rather than reusing this function.
func buildCreateSeed(result orchestrator.CreateResult) conversation.Data {
	rows := make([]movementRow, 0, len(result.Movements))
	var categoryGaps, accountGaps []string

	for i, draft := range result.Movements {
		row := movementRow{
			Type:             draft.Type,
			Amount:           draft.Amount,
			Currency:         draft.Currency,
			AccountNameGuess: draft.AccountNameGuess,
			Category:         draft.Category,
			Subcategory:      draft.Subcategory,
			PaymentMethod:    draft.PaymentMethod,
			Merchant:         draft.Merchant,
			Description:      draft.Description,
			Date:             draft.Date,
		}
		if draft.AccountID != nil {
			row.AccountID = strconv.FormatUint(*draft.AccountID, 10)
		}

		idx := strconv.Itoa(i)
		if draft.Category == "PENDING_REVIEW" {
			categoryGaps = append(categoryGaps, idx)
		}
		if draft.Type == "transfer" && draft.AccountID == nil {
			accountGaps = append(accountGaps, idx)
		}

		rows = append(rows, row)
	}

	return conversation.Data{
		"mode":                  "create",
		"old_movement_ids":      encodeStringSlice(nil),
		"movements":             encodeMovementRows(rows),
		"pending_category_gaps": encodeStringSlice(categoryGaps),
		"pending_account_gaps":  encodeStringSlice(accountGaps),
	}
}

// parseUintSlice turns the string-encoded movement IDs carried through
// conversation.Data back into real uint IDs, for repository calls that
// take []uint (SoftDeleteByIDs, ReplaceMovements).
func parseUintSlice(ids []string) ([]uint, error) {
	out := make([]uint, 0, len(ids))
	for _, s := range ids {
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, uint(v))
	}
	return out, nil
}
