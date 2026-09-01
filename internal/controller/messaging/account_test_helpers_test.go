package messaging

import (
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
)

func acct(id uint64, cur currency.Currency, def bool) account.Account {
	return account.Account{Model: gorm.Model{ID: uint(id)}, UserID: 1, Currency: cur, IsDefault: def}
}
