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

// accountsMap indexa cuentas igual que accountIndex, para los tests que arman
// los mapas a mano en vez de pasar por loadAccountIndex.
func accountsMap(accs ...account.Account) (map[uint64]account.Account, map[string]uint64) {
	byID := map[uint64]account.Account{}
	def := map[string]uint64{}
	for _, a := range accs {
		byID[uint64(a.ID)] = a
		if a.IsDefault {
			def[a.Currency.String()] = uint64(a.ID)
		}
	}
	return byID, def
}
