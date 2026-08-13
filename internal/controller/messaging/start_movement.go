package messaging

import (
	"context"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// needsFirstAccount is true when a non-transfer row has no account_id and the
// user has no default in that row's currency (zero-account first run) — the
// lazy-create trigger for stepCreateFirstAccount.
func needsFirstAccount(seed conversation.Data, has func(cur currency.Currency) bool) bool {
	return firstAccountCurrency(seed, has) != ""
}

// hasDefaultFor arma el predicado que needsFirstAccount/firstAccountCurrency
// piden. Existe porque los tres call sites del flujo lo escribían idéntico y
// nada garantizaba que siguieran de acuerdo.
func hasDefaultFor(accounts accountRepository, data conversation.Data) func(currency.Currency) bool {
	return func(cur currency.Currency) bool {
		return accounts.HasDefaultForCurrency(data.UserID(), cur)
	}
}

// firstAccountCurrency devuelve la moneda de la fila que dispara el alta, o ""
// si ninguna la dispara. Es la misma decisión que needsFirstAccount —de ahí que
// aquélla delegue acá— pero devolviendo el DATO en vez del sí/no.
//
// Hace falta porque el default de cuenta es POR MONEDA (FindDefaultByCurrency,
// UnsetDefault(userID, currency)). Un prompt que dice "tu cuenta principal" sin
// nombrar la moneda es ambiguo con dos cuentas y directamente falso con dos
// monedas: la nueva cuenta en USD no va a recibir ningún movimiento en pesos.
func firstAccountCurrency(seed conversation.Data, has func(cur currency.Currency) bool) string {
	for _, r := range decodeMovementRows(seed) {
		if movement.TypeFromString(r.Type) == movement.Transfer {
			continue
		}
		if r.AccountID == "" && !has(currency.Currency(r.Currency)) {
			return r.Currency
		}
	}
	return ""
}

// startMovementDeleteFlowFor seeds and starts movement_delete.
// resolvedIndex >= 0 means exactly one candidate is already known (lets
// the flow skip its picker step); -1 means show the picker over every
// candidate.
func (c *controller) startMovementDeleteFlowFor(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, candidates []transactionGroup, resolvedIndex int) error {
	labels := make([]string, 0, len(candidates))
	for _, g := range candidates {
		labels = append(labels, candidateLabel(g))
	}

	seed := conversation.Data{
		keyCandidateLabels: encodeStringSlice(labels),
		keyCandidateGroups: encodeCandidateGroups(candidates),
	}
	if resolvedIndex >= 0 {
		seed[keyResolvedIndex] = strconv.Itoa(resolvedIndex)
	}

	return c.startFlow(ctx, b, chatID, userID, movementDeleteFlowName, seed, "delete: start movement_delete flow")
}
