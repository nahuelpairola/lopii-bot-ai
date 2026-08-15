package flow

import (
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/subcategory"
)

// Las interfaces de repositorio de este paquete son SOLO lo que los builders
// necesitan para armar opciones/preguntas. Son unexported a propósito: los
// constructores se llaman desde server.go con tipos concretos, que satisfacen
// estructuralmente. El borde (messaging) define sus propias interfaces para
// sus finish/start — nunca se importan tipos concretos entre paquetes.

// accountRepository: resolver defaults por moneda y listar cuentas del usuario.
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

// balanceSummer es la lectura angosta que account_manage necesita para mostrar
// el saldo actual.
type balanceSummer interface {
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
}

// categoryLister es lo mínimo para armar un picker de categorías.
type categoryLister interface {
	DistinctCategoriesForUser(userID uint64) ([]string, error)
	IconForCategory(userID uint64, category string) string
}

// ownedSubcategoryLister es lo que el flujo 1 de category_manage necesita: las
// filas que creó el usuario (las globales no son candidatas a borrarse).
type ownedSubcategoryLister interface {
	FindOwnedByUser(userID uint64) ([]subcategory.Subcategory, error)
}

// targetSubcategoryLister es lo que el flujo 2 necesita para sus dos pickers.
type targetSubcategoryLister interface {
	categoryLister
	FindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
}
