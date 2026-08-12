package messaging

import (
	"errors"
	"fmt"

	"lopiibot.com/internal/constants"
)

// changeField y changeOp son el vocabulario de una corrección estructurada: el
// modelo emite un CAMBIO, no la fila entera corregida.
//
// Pedirle que reproduzca ocho filas completas para tocar un campo es donde los
// modelos corrompen datos en silencio, y una description o una fecha alterada no
// las ataja ningún guard. Un diff no puede corromper lo que no menciona.
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

// correctionChange es un cambio a un campo. Value viaja SIEMPRE como string:
// conversation.Data round-trippea por JSONB y un número vuelve float64.
type correctionChange struct {
	Field changeField `json:"field"`
	Op    changeOp    `json:"op"`
	Value string      `json:"value"`
}

var (
	// errOpNotForField: sólo amount acepta aritmética. Sin esto, un `add` sobre
	// currency o date compila, corre y hace cualquier cosa.
	errOpNotForField = errors.New("correction: ese campo sólo acepta set")
	// errRefundExceedsAmount: un reintegro más grande que la compra daría vuelta
	// el signo, y Normalize lo re-firmaría como ingreso. Es un error del usuario
	// o un mal parseo; nunca un cambio de tipo silencioso.
	errRefundExceedsAmount = errors.New("correction: el reintegro supera el monto")
	// errUnparseableValue: NUNCA coercionar a cero. Coercionar a cero es cómo un
	// "no entendí" se convierte en un borrado (ver correctionIsDeletion).
	errUnparseableValue = errors.New("correction: valor ilegible")
	// errTransferNotACorrection: un transfer son dos patas y una contraparte, así
	// que convertir un gasto en transferencia es un delete + create, no un cambio.
	errTransferNotACorrection = errors.New("correction: transfer no es una corrección")
)

// applyChange aplica UN cambio a UNA fila y devuelve la fila corregida. Nunca
// muta la de entrada.
//
// La aritmética vive acá y no en el modelo: el modelo dice "restá 100", la app
// calcula el 900. Esa es la misma regla que ya gobierna el signo — la app posee
// la escritura, el modelo posee la interpretación.
func applyChange(row movementRow, ch correctionChange) (movementRow, error) {
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
		// El usuario dice "proyecto hogar/agua" como UNA cosa. El par lo resuelve
		// el gap contra `known`, igual que en CREATE; partirlo acá sería pedirle
		// al modelo que divida algo que el usuario no dividió.
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

func applyAmountChange(row movementRow, ch correctionChange) (movementRow, error) {
	cur, err := parseARAmount(row.Amount)
	if err != nil {
		return row, errUnparseableValue
	}
	val, err := parseARAmount(ch.Value)
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
		// 2 decimales, half-up. Un redondeo sin definir en un camino de plata es
		// un bug esperando el primer monto impar.
		out = cur.Mul(val).Round(2)
	default:
		return row, fmt.Errorf("correction: operación desconocida %q", ch.Op)
	}

	row.Amount = out.String()
	return row, nil
}
