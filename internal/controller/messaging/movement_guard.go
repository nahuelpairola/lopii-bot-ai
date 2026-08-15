package messaging

import "lopiibot.com/internal/movement"

// insufficientFunds NO es del dominio: es cómo este controller se avisa a sí
// mismo que tiene que desviar al gate de confirmación en vez de insertar. El
// dominio solo reporta los shortfalls (movement.CheckBalances); qué hacer con
// ellos es una decisión de UI.
type insufficientFunds struct {
	shortfalls []movement.AccountShortfall
}

func (e *insufficientFunds) Error() string { return "insufficient funds" }
