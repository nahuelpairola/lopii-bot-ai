package messaging

import (
	"maps"
	"strconv"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
)

// accountPendingCreate is the sentinel movementRow.AccountID value
// meaning "the user chose, mid-flow, to create this account" — real
// account ids are always numeric strings, so this can never collide.
const accountPendingCreate = "PENDING_CREATE"

// mode discriminates buildCreateSeed's flow: a fresh CREATE vs a resolved
// UPDATE reusing the create flow. Stored under keyMode.
const (
	modeCreate = "create"
	modeUpdate = "update"
)

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
	AccountName      string
	Category         string
	Subcategory      string
	PaymentMethod    string
	Merchant         string
	Description      string
	Date             string
	Icon             string
	Group            string
}

func stringOrEmpty(v any) string {
	s, _ := v.(string)
	return s
}

// movementGapDescriptor names a movementRow for the gap-fill ask-prompts, so
// a compound message with several pending rows never asks two identical
// questions in a row — merchant is preferred (concrete: "en Coto"),
// description is the fallback (description is a required Call 2 CREATE
// field — always populated, see orchestrator.MovementDraft).
func movementGapDescriptor(row movementRow) string {
	detail := row.Merchant
	if detail == "" {
		detail = row.Description
	}
	return "$" + row.Amount + " · " + detail
}

// copyData es maps.Clone con una garantía extra: el resultado nunca es nil.
// `maps.Clone(nil)` devuelve nil y todos los call sites escriben sobre la copia,
// así que sin la guarda un Data nil (posible: `data: null` en JSONB deserializa
// a nil sin error) haría panic. Se queda como wrapper por los ~36 call sites.
func copyData(data conversation.Data) conversation.Data {
	if data == nil {
		return conversation.Data{}
	}
	return maps.Clone(data)
}

func decodeMovementRows(data conversation.Data) []movementRow {
	raw, _ := data[keyMovements].([]interface{})
	rows := make([]movementRow, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]interface{})
		rows = append(rows, movementRow{
			Type:             stringOrEmpty(m[keyRowType]),
			Amount:           stringOrEmpty(m[keyRowAmount]),
			Currency:         stringOrEmpty(m[keyCurrency]),
			AccountID:        stringOrEmpty(m[keyAccountID]),
			AccountNameGuess: stringOrEmpty(m[keyAccountNameGuess]),
			AccountName:      stringOrEmpty(m[keyAccountName]),
			Category:         stringOrEmpty(m[keyCategory]),
			Subcategory:      stringOrEmpty(m[keySubcategory]),
			PaymentMethod:    stringOrEmpty(m[keyPaymentMethod]),
			Merchant:         stringOrEmpty(m[keyMerchant]),
			Description:      stringOrEmpty(m[keyDescription]),
			Date:             stringOrEmpty(m[keyDate]),
			Icon:             stringOrEmpty(m[keyIcon]),
			Group:            stringOrEmpty(m[keyGroup]),
		})
	}
	return rows
}

func encodeMovementRows(rows []movementRow) []interface{} {
	encoded := make([]interface{}, 0, len(rows))
	for _, r := range rows {
		encoded = append(encoded, map[string]interface{}{
			keyRowType:          r.Type,
			keyRowAmount:        r.Amount,
			keyCurrency:         r.Currency,
			keyAccountID:        r.AccountID,
			keyAccountNameGuess: r.AccountNameGuess,
			keyAccountName:      r.AccountName,
			keyCategory:         r.Category,
			keySubcategory:      r.Subcategory,
			keyPaymentMethod:    r.PaymentMethod,
			keyMerchant:         r.Merchant,
			keyDescription:      r.Description,
			keyDate:             r.Date,
			keyIcon:             r.Icon,
			keyGroup:            r.Group,
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
func buildCreateSeed(result orchestrator.CreateResult, taxonomy []orchestrator.TaxonomyEntry) conversation.Data {
	rows := make([]movementRow, 0, len(result.Movements))
	var categoryGaps, accountGaps []string

	// known indexa los pares (categoría, subcategoría) que el usuario realmente
	// tiene. El modelo debería devolver PENDING_REVIEW cuando duda, pero a veces
	// inventa un par que no existe; sin este set eso no marcaba gap, el flujo
	// insertaba derecho y FindByCategoryAndSubcategory fallaba — el movimiento se
	// perdía con un error genérico (causa de los create_failed en intent_events).
	// Taxonomía vacía = no validar: sin con qué comparar, no se inventan gaps.
	known := make(map[string]bool, len(taxonomy))
	for _, t := range taxonomy {
		known[t.Category+"\x00"+t.Subcategory] = true
	}

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
			Group:            draft.Group,
		}
		if draft.AccountID != nil {
			row.AccountID = strconv.FormatUint(*draft.AccountID, 10)
		}

		idx := strconv.Itoa(i)
		if draft.Category == constants.PendingReview || (len(known) > 0 && !known[draft.Category+"\x00"+draft.Subcategory]) {
			categoryGaps = append(categoryGaps, idx)
		}
		// Un gap de cuenta significa "no puedo saber a qué cuenta va esta fila y
		// tengo que preguntar". Lo abren dos casos: una transferencia con una pata
		// sin resolver, y cualquier fila que nombró una cuenta que el modelo no pudo
		// matchear con una existente. Sin el segundo caso esa fila cae callada en la
		// cuenta default de la moneda y la cuenta nombrada nunca se crea — la
		// intención declarada por el usuario se descarta.
		//
		// Una fila sin AccountNameGuess y sin AccountID NO es un gap: es el camino
		// normal "usá mi default" y tiene que seguir siendo mudo.
		if draft.AccountID == nil && (draft.Type == constants.Transfer || draft.AccountNameGuess != "") {
			accountGaps = append(accountGaps, idx)
		}

		rows = append(rows, row)
	}

	return conversation.Data{
		keyMode:                modeCreate,
		keyOldMovementIDs:      encodeStringSlice(nil),
		keyMovements:           encodeMovementRows(rows),
		keyPendingCategoryGaps: encodeStringSlice(categoryGaps),
		keyPendingAccountGaps:  encodeStringSlice(accountGaps),
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
