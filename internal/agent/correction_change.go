package agent

import (
	"errors"
	"fmt"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/movement"
)

type changeField string
type changeOp string

const (
	fieldCategory    changeField = "category"
	fieldAccount     changeField = "account"
	fieldDate        changeField = "date"
	fieldAmount      changeField = "amount"
	fieldCurrency    changeField = "currency"
	fieldDescription changeField = "description"
	fieldType        changeField = "type"

	opSet      changeOp = "set"
	opAdd      changeOp = "add"
	opSubtract changeOp = "subtract"
	opMultiply changeOp = "multiply"
)

type correctionChange struct {
	Field changeField `json:"field"`
	Op    changeOp    `json:"op"`
	Value string      `json:"value"`
}

var (
	errOpNotForField          = errors.New("correction: ese campo sólo acepta set")
	errRefundExceedsAmount    = errors.New("correction: el reintegro supera el monto")
	errUnparseableValue       = errors.New("correction: valor ilegible")
	errTransferNotACorrection = errors.New("correction: transfer no es una corrección")
)

func defaultChangeOps(changes []correctionChange) {
	for i := range changes {
		if changes[i].Op == "" {
			changes[i].Op = opSet
		}
	}
}

func applyChange(row movement.MovementRow, ch correctionChange) (movement.MovementRow, error) {
	if ch.Op != opSet && ch.Field != fieldAmount {
		return row, fmt.Errorf("%w: %s con %s", errOpNotForField, ch.Field, ch.Op)
	}

	switch ch.Field {
	case fieldAmount:
		return applyAmountChange(row, ch)
	case fieldType:
		if ch.Value == constants.Transfer {
			return row, errTransferNotACorrection
		}
		row.Type = ch.Value
	case fieldCategory:
		row.Category = ch.Value
		row.Subcategory = ""
	case fieldAccount:
		row.AccountNameGuess = ch.Value
		row.AccountID = ""
	case fieldDate:
		row.Date = ch.Value
	case fieldCurrency:
		row.Currency = ch.Value
	case fieldDescription:
		row.Description = ch.Value
	default:
		return row, fmt.Errorf("correction: campo desconocido %q", ch.Field)
	}
	return row, nil
}

func applyAmountChange(row movement.MovementRow, ch correctionChange) (movement.MovementRow, error) {
	cur, err := movement.ParseARAmount(row.Amount)
	if err != nil {
		return row, errUnparseableValue
	}
	val, err := movement.ParseARAmount(ch.Value)
	if err != nil {
		return row, errUnparseableValue
	}

	var out = val
	switch ch.Op {
	case opSet:
		out = val
	case opAdd:
		out = cur.Add(val)
	case opSubtract:
		out = cur.Sub(val)
		if out.IsNegative() {
			return row, fmt.Errorf("%w: %s sobre %s", errRefundExceedsAmount, val, cur)
		}
	case opMultiply:
		out = cur.Mul(val).Round(2)
	default:
		return row, fmt.Errorf("correction: operación desconocida %q", ch.Op)
	}

	row.Amount = out.String()
	return row, nil
}
