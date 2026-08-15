package flow

import (
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// runner es la dependencia mínima que el código de escritura de movimientos
// (movement_write.go) necesita del borde (messaging). El tipo es unexported a
// propósito; los MÉTODOS tienen que ser exportados porque una interfaz con
// métodos unexported solo la pueden implementar tipos del mismo paquete, y acá
// la implementa el *controller del borde. Ver el comentario de repos.go.
type runner interface {
	FindUserAccounts(userID uint64) ([]account.Account, error)
	InsertAccount(*account.Account) error
	GetAccount(id uint64) (*account.Account, error)
	SumAmountForAccount(id uint64) (decimal.Decimal, error)
	InsertMovements(movs []movement.Movement) error
	ReplaceMovements(oldIDs []uint, movs []movement.Movement) error
	FindSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
}
