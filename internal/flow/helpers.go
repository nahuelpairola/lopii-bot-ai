package flow

import (
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

func NeedsFirstAccount(seed conversation.Data, has func(cur currency.Currency) bool) bool {
	return FirstAccountCurrency(seed, has) != ""
}

func HasDefaultFor(accounts accountRepository, data conversation.Data) func(currency.Currency) bool {
	return func(cur currency.Currency) bool {
		return accounts.HasDefaultForCurrency(data.UserID(), cur)
	}
}

func FirstAccountCurrency(seed conversation.Data, has func(cur currency.Currency) bool) string {
	for _, r := range movement.DecodeMovementRows(seed) {
		if movement.TypeFromString(r.Type) == movement.Transfer {
			continue
		}
		if r.AccountID == "" && !has(currency.Currency(r.Currency)) {
			return r.Currency
		}
	}
	return ""
}
