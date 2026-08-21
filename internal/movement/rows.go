package movement

import (
	"github.com/shopspring/decimal"

	"lopiibot.com/internal/conversation"
)

// MovementRow is the JSON-safe, per-row shape carried inside
// conversation.Data during CREATE's gap-fill flow. Every field is a
// string: conversation.Data round-trips through Postgres JSONB, and
// only strings/bools/slices/maps of those survive that round-trip
// without corruption (numbers decode back as float64 — see
// conversation.Data.UserID's own float64 fallback for why).
type MovementRow struct {
	Type             string
	Amount           string
	Currency         string
	AccountID        string
	AccountNameGuess string
	AccountName      string
	Category         string
	Subcategory      string
	PaymentMethod    string
	Description      string
	Date             string
	Icon             string
	Group            string
	// TransferOut marca la pata que SALE de una transferencia. La lleva la app,
	// nunca el modelo: Amount es siempre el valor absoluto (el signo guardado no
	// sale de storage), así que sin esta marca una transferencia reescrita vuelve
	// con las dos patas en positivo y validateTransferGroups rechaza el grupo.
	TransferOut string
}

// TransferOutMark es el único valor con significado en MovementRow.TransferOut.
// Es un string porque toda la fila lo es: Data va y vuelve de una columna JSONB.
const TransferOutMark = "true"

// RowAmount lee el monto de una fila y le devuelve el signo contable que la app
// posee. Sólo hace falta para la pata que SALE de una transferencia reescrita:
// el resto de los tipos los firma Normalize, y una fila de CREATE trae el signo
// que clasificó el modelo.
func RowAmount(row MovementRow) (decimal.Decimal, error) {
	amount, err := ParseARAmount(row.Amount)
	if err != nil {
		return decimal.Zero, err
	}
	if row.TransferOut == TransferOutMark {
		return amount.Abs().Neg(), nil
	}
	return amount, nil
}

// MovementGapDescriptor names a MovementRow for the gap-fill ask-prompts, so
// a compound message with several pending rows never asks two identical
// questions in a row. La description es un campo requerido del Call 2 CREATE
// —siempre viene poblada, ver orchestrator.MovementDraft—, así que desde el
// fold de merchant es la única fuente y no hace falta fallback.
func MovementGapDescriptor(row MovementRow) string {
	return "$" + row.Amount + " · " + row.Description
}

// DecodeMovementRows read the "movements" key of Data, which was stored as
// []interface{} of map[string]interface{} (the JSONB shape), back into rows.
func DecodeMovementRows(data conversation.Data) []MovementRow {
	raw, _ := data[conversation.KeyMovements].([]interface{})
	rows := make([]MovementRow, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]interface{})
		rows = append(rows, MovementRow{
			Type:             conversation.StringOrEmpty(m[conversation.KeyRowType]),
			Amount:           conversation.StringOrEmpty(m[conversation.KeyRowAmount]),
			Currency:         conversation.StringOrEmpty(m[conversation.KeyCurrency]),
			AccountID:        conversation.StringOrEmpty(m[conversation.KeyAccountID]),
			AccountNameGuess: conversation.StringOrEmpty(m[conversation.KeyAccountNameGuess]),
			AccountName:      conversation.StringOrEmpty(m[conversation.KeyAccountName]),
			Category:         conversation.StringOrEmpty(m[conversation.KeyCategory]),
			Subcategory:      conversation.StringOrEmpty(m[conversation.KeySubcategory]),
			PaymentMethod:    conversation.StringOrEmpty(m[conversation.KeyPaymentMethod]),
			Description:      conversation.StringOrEmpty(m[conversation.KeyDescription]),
			Date:             conversation.StringOrEmpty(m[conversation.KeyDate]),
			Icon:             conversation.StringOrEmpty(m[conversation.KeyIcon]),
			Group:            conversation.StringOrEmpty(m[conversation.KeyGroup]),
			TransferOut:      conversation.StringOrEmpty(m[conversation.KeyTransferOut]),
		})
	}
	return rows
}

// EncodeMovementRows stores rows as the []interface{} of map[string]interface{}
// shape that survives the JSONB round-trip.
func EncodeMovementRows(rows []MovementRow) []interface{} {
	encoded := make([]interface{}, 0, len(rows))
	for _, r := range rows {
		encoded = append(encoded, map[string]interface{}{
			conversation.KeyRowType:          r.Type,
			conversation.KeyRowAmount:        r.Amount,
			conversation.KeyCurrency:         r.Currency,
			conversation.KeyAccountID:        r.AccountID,
			conversation.KeyAccountNameGuess: r.AccountNameGuess,
			conversation.KeyAccountName:      r.AccountName,
			conversation.KeyCategory:         r.Category,
			conversation.KeySubcategory:      r.Subcategory,
			conversation.KeyPaymentMethod:    r.PaymentMethod,
			conversation.KeyDescription:      r.Description,
			conversation.KeyDate:             r.Date,
			conversation.KeyIcon:             r.Icon,
			conversation.KeyGroup:            r.Group,
			conversation.KeyTransferOut:      r.TransferOut,
		})
	}
	return encoded
}
