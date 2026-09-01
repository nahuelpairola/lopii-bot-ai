package server

import (
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/subcategory"
)

type flowAccountRepository interface {
	FindDefaultByCurrency(userID uint64, currency currency.Currency) (*account.Account, error)
	HasDefaultForCurrency(userID uint64, currency currency.Currency) bool
	FindByUserID(userID uint64) ([]account.Account, error)
}

type flowBalanceSummer interface {
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
}

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
