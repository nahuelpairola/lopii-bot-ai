package server

import (
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/subcategory"
)

// Las interfaces de los builders de flow son unexported: se satisfacen
// estructuralmente desde acá, con los tipos concretos. Estas dos son el reflejo
// mínimo de lo que piden (flow/repos.go), declaradas del lado del consumidor
// como manda la convención del repo — account y movement devuelven tipos
// unexported, así que nombrarlos no es opción.

type flowAccountRepository interface {
	FindDefaultByCurrency(userID uint64, currency currency.Currency) (*account.Account, error)
	HasDefaultForCurrency(userID uint64, currency currency.Currency) bool
	FindByUserID(userID uint64) ([]account.Account, error)
}

type flowBalanceSummer interface {
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
}

// registerFlows arma el engine con los 15 flujos.
//
// Este es uno de los TRES lugares que hay que tocar para agregar un flujo —los
// otros dos están en flow/CLAUDE.md— y es el que se olvida: un flujo sin
// registrar compila, arranca, y recién falla cuando alguien lo dispara, con un
// "flow is not registered" que no dice qué falta.
//
// El orden no importa: el engine los indexa por nombre.
func registerFlows(
	engine *conversation.Engine,
	subcategories *subcategory.Cache,
	accounts flowAccountRepository,
	movements flowBalanceSummer,
) {
	engine.Register(flow.NewMovementCreateFlow(subcategories, accounts))
	engine.Register(flow.NewMovementUpdatePickFlow())
	engine.Register(flow.NewMovementUpdateConfirmFlow())
	engine.Register(flow.NewMovementDeleteFlow())
	engine.Register(flow.NewAccountCreateFlow())
	engine.Register(flow.NewAccountManageFlow(movements))
	engine.Register(flow.NewAccountMoveOfferFlow())
	engine.Register(flow.NewSubcategorySetupFlow(subcategories))
	engine.Register(flow.NewCategoryMatchOfferFlow())
	engine.Register(flow.NewCategoryProposalConfirmFlow())
	engine.Register(flow.NewCategoryManagePickFlow(subcategories))
	engine.Register(flow.NewCategoryManageTargetFlow(subcategories))
	engine.Register(flow.NewMovementNegativeConfirmFlow())
	engine.Register(flow.NewReminderSetupFlow())
	engine.Register(flow.NewAskUserFlow())
}
