package messaging

import "lopiibot.com/internal/movement"

// Los invariantes del modelo de plata (Normalize, AssignTransactionIDs,
// CheckBalances) viven en internal/movement, al lado del tipo que protegen.
// Acá quedan solo los alias que usan los call sites de este paquete y el tipo
// de control de flujo que no es del dominio.
var (
	errZeroAmount              = movement.ErrZeroAmount
	errCurrencyAccountMismatch = movement.ErrCurrencyAccountMismatch
	errTransferLeg             = movement.ErrTransferLeg
	errNoAccountForCurrency    = movement.ErrNoAccountForCurrency
)

// insufficientFunds NO es del dominio: es cómo este controller se avisa a sí
// mismo que tiene que desviar al gate de confirmación en vez de insertar. El
// dominio solo reporta los shortfalls (movement.CheckBalances); qué hacer con
// ellos es una decisión de UI.
type insufficientFunds struct {
	shortfalls []movement.AccountShortfall
}

func (e *insufficientFunds) Error() string { return "insufficient funds" }
