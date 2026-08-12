package messaging

import (
	"errors"
	"fmt"
	"strings"

	"lopiibot.com/internal/constants"
)

var (
	// errAmbiguousSetAll: "poné todos en 1500" no es algo que un usuario quiera
	// decir; es un scope mal completado. Preguntar cuál es mejor que aplanar n
	// montos distintos al mismo número.
	errAmbiguousSetAll = errors.New("correction: set de monto sobre varios movimientos")
	// errAccountOnTransfer: "cuál pata" no es expresable en este schema, y
	// ReassignAccount repunta patas en SQL crudo SIN revalidar.
	errAccountOnTransfer = errors.New("correction: no se puede cambiar la cuenta de una transferencia")
	// errRefundToOtherAccount: el reintegro que entró en otra cuenta es un
	// INGRESO, no una corrección. Restarlo del gasto original deja DOS saldos mal.
	errRefundToOtherAccount = errors.New("correction: el reintegro entró en otra cuenta")
)

// guardContext es lo que la app sabe del mensaje y que las guardas necesitan.
// No lo provee el modelo: son datos, no interpretación.
type guardContext struct {
	// NamedAccount: la cuenta que el mensaje nombró, si nombró alguna.
	NamedAccount string
	// Scope: "one" | "all", tal como lo emitió el modelo.
	Scope string
}

const (
	scopeOne = "one"
	scopeAll = "all"
)

// applyChanges aplica el set de cambios a las filas de UN grupo.
//
// Un grupo de transferencia son dos patas que DEBEN sumar cero. Un cambio
// aplicado ingenuamente lo rompe: {amount, set, 5000} sobre las dos da +10.000,
// y sobre una sola deja un grupo que no balancea. El guard lo rechaza al
// insertar, así que la corrección fallaría — ruidosa pero inútil.
func applyChanges(rows []movementRow, changes []correctionChange) ([]movementRow, error) {
	if len(rows) == 0 {
		return nil, errors.New("correction: no hay filas que corregir")
	}
	isTransfer := isTransferGroup(rows)

	out := make([]movementRow, len(rows))
	copy(out, rows)

	for _, ch := range changes {
		if isTransfer && ch.Field == fieldAccount {
			return nil, errAccountOnTransfer
		}
		for i := range out {
			applied, err := applyChange(out[i], ch)
			if err != nil {
				return nil, fmt.Errorf("fila %d: %w", i, err)
			}
			out[i] = applied
		}
	}
	return out, nil
}

// isTransferGroup: dos patas o más, todas tipadas transfer.
func isTransferGroup(rows []movementRow) bool {
	if len(rows) < 2 {
		return false
	}
	for _, r := range rows {
		if r.Type != constants.Transfer {
			return false
		}
	}
	return true
}

// applyChangesToSet corre las guardas que dependen del CONJUNTO y del mensaje,
// no de una fila suelta. Ninguna es una instrucción que el modelo tenga que
// recordar: todas son chequeos que la app puede hacer sola.
func applyChangesToSet(groups [][]movementRow, changes []correctionChange, ctx guardContext) ([][]movementRow, error) {
	if len(groups) == 0 {
		return nil, errors.New("correction: no hay grupos que corregir")
	}

	for _, ch := range changes {
		if ch.Field == fieldAmount && ch.Op == opSet && ctx.Scope == scopeAll && len(groups) > 1 {
			return nil, errAmbiguousSetAll
		}
		if err := guardRefundAccount(groups, ch, ctx); err != nil {
			return nil, err
		}
	}

	out := make([][]movementRow, 0, len(groups))
	for _, g := range groups {
		applied, err := applyChanges(g, changes)
		if err != nil {
			return nil, err
		}
		out = append(out, applied)
	}
	return out, nil
}

// guardRefundAccount es el agujero contable de la §5.3.4, y NO puede vivir en el
// gate de casi-duplicado: ese exige misma cuenta, y este caso se define por NO
// tenerla.
//
// La regla vigente decía "el amount corregido es el original MENOS lo devuelto",
// y eso sólo es cierto si la plata volvió a la MISMA cuenta. Disney pagado con
// Galicia y reintegrado a Mercado Pago: restarlo del gasto original sube el
// saldo de Galicia y deja Mercado Pago intacto. Los dos saldos quedan mal.
func guardRefundAccount(groups [][]movementRow, ch correctionChange, ctx guardContext) error {
	if ch.Field != fieldAmount || (ch.Op != opSubtract && ch.Op != opMultiply) {
		return nil
	}
	if ctx.NamedAccount == "" {
		return nil // el default abrumador: el usuario no dice a dónde volvió
	}
	for _, g := range groups {
		for _, r := range g {
			if r.AccountName != "" && !strings.EqualFold(r.AccountName, ctx.NamedAccount) {
				return fmt.Errorf("%w: %s, no %s", errRefundToOtherAccount, ctx.NamedAccount, r.AccountName)
			}
		}
	}
	return nil
}
