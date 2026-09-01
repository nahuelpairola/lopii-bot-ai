package orchestrator

import "github.com/shopspring/decimal"

const (
	catSistema       = "Sistema"
	subTransferencia = "Transferencia"
	catInversiones   = "Inversiones"
	subDolares       = "Dólares"
)

func StructuralPair(rows []MovementDraft) (category, subcategory string, ok bool) {
	if len(rows) != 2 {
		return "", "", false
	}
	a, errA := decimal.NewFromString(rows[0].Amount)
	b, errB := decimal.NewFromString(rows[1].Amount)
	if errA != nil || errB != nil {
		return "", "", false
	}

	if rows[0].Currency != rows[1].Currency {
		return catInversiones, subDolares, true
	}
	if a.Abs().Equal(b.Abs()) {
		return catSistema, subTransferencia, true
	}
	return "", "", false
}
