package flow

import (
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/subcategory"
)

type accountRepository interface {
	FindDefaultByCurrency(userID uint64, currency currency.Currency) (*account.Account, error)
	HasDefaultForCurrency(userID uint64, currency currency.Currency) bool
	FindByUserID(userID uint64) ([]account.Account, error)
}

type subcategoryRepository interface {
	FindByCategoryAndSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
	FindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	DistinctCategoriesForUser(userID uint64) ([]string, error)
	IconForCategory(userID uint64, category string) string
}

type balanceSummer interface {
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
}

type categoryLister interface {
	DistinctCategoriesForUser(userID uint64) ([]string, error)
	IconForCategory(userID uint64, category string) string
}

type ownedSubcategoryLister interface {
	FindOwnedByUser(userID uint64) ([]subcategory.Subcategory, error)
}

type targetSubcategoryLister interface {
	categoryLister
	FindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
}
