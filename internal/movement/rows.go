package movement

import (
	"github.com/shopspring/decimal"

	"lopiibot.com/internal/conversation"
)

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
	TransferOut      string
}

const TransferOutMark = "true"

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

func MovementGapDescriptor(row MovementRow) string {
	return "$" + row.Amount + " · " + row.Description
}

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
