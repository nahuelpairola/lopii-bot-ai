package orchestrator

import "github.com/shopspring/decimal"

// Los pares que hoy viven como strings literales adentro de movementPatternRules
// para que el modelo los copie — y por lo tanto también para que los copie MAL.
const (
	catSistema       = "Sistema"
	subTransferencia = "Transferencia"
	catInversiones   = "Inversiones"
	subDolares       = "Dólares"
)

// structuralPair devuelve el par que se deduce de la FORMA del movimiento,
// junto con si es deducible.
//
// **Es un DEFAULT, no una regla dura**, y la diferencia la decidió la taxonomía
// real: `Inversiones | FCI` existe como par propio ("Suscripción o rescate de
// fondos comunes de inversión"). Una suscripción de FCI entre dos cuentas
// propias en la misma moneda es un grupo de 2 patas que suma cero —
// exactamente la forma de una transferencia— y NO es una transferencia. Sólo el
// mensaje distingue una de otra.
//
// Así que el clasificador puede pisar esto. Lo que el default compra igual es
// que el caso abrumador (una transferencia de verdad) no gaste una decisión del
// modelo, y que el par salga bien escrito de la app en vez de copiado a mano.
func structuralPair(rows []MovementDraft) (category, subcategory string, ok bool) {
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
	// Misma moneda: tiene que balancear. Los montos llegan del modelo en
	// positivo (el signo lo pone la app), así que "suma cero" es "son iguales".
	if a.Abs().Equal(b.Abs()) {
		return catSistema, subTransferencia, true
	}
	return "", "", false
}
