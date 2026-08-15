package flow

import (
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// NeedsFirstAccount is true when a non-transfer row has no account_id and the
// user has no default in that row's currency (zero-account first run) — the
// lazy-create trigger for stepCreateFirstAccount.
func NeedsFirstAccount(seed conversation.Data, has func(cur currency.Currency) bool) bool {
	return FirstAccountCurrency(seed, has) != ""
}

// HasDefaultFor arma el predicado que NeedsFirstAccount/FirstAccountCurrency
// piden. Existe porque los tres call sites del flujo lo escribían idéntico y
// nada garantizaba que siguieran de acuerdo.
func HasDefaultFor(accounts accountRepository, data conversation.Data) func(currency.Currency) bool {
	return func(cur currency.Currency) bool {
		return accounts.HasDefaultForCurrency(data.UserID(), cur)
	}
}

// FirstAccountCurrency devuelve la moneda de la fila que dispara el alta, o ""
// si ninguna la dispara. Es la misma decisión que NeedsFirstAccount —de ahí que
// aquélla delegue acá— pero devolviendo el DATO en vez del sí/no.
//
// Hace falta porque el default de cuenta es POR MONEDA (FindDefaultByCurrency,
// UnsetDefault(userID, currency)). Un prompt que dice "tu cuenta principal" sin
// nombrar la moneda es ambiguo con dos cuentas y directamente falso con dos
// monedas: la nueva cuenta en USD no va a recibir ningún movimiento en pesos.
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
