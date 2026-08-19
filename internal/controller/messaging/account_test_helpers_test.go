package messaging

import (
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
)

// acct arma una cuenta mínima para los tests. Vivía en movement_guard_test.go;
// cuando el guard se mudó a internal/movement el helper quedó acá, que es donde
// lo usan los tests que siguen siendo de este paquete.
func acct(id uint64, cur currency.Currency, def bool) account.Account {
	return account.Account{Model: gorm.Model{ID: uint(id)}, UserID: 1, Currency: cur, IsDefault: def}
}
