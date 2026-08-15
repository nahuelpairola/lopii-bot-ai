package flow

import (
	"context"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

// runner es la dependencia mínima que el código de escritura de movimientos
// (movement_write.go) y los finishes migrados (movement_finish.go,
// account_finish.go) necesitan del borde (messaging). El tipo es unexported a
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

	RenameAccount(id uint64, name string) error
	FindDefaultAccountByCurrency(userID uint64, cur currency.Currency) (*account.Account, error)
	UnsetDefaultAccount(userID uint64, cur currency.Currency) error
	SetDefaultAccount(id uint64) error
	InsertMovementsBatch(movs []movement.Movement) error
	ReassignAccountMovements(fromID, toID uint64) error

	SubcategoryIconForCategory(userID uint64, category string) string
	InsertSubcategory(*subcategory.Subcategory) error
	ReloadSubcategories() error
	DeleteSubcategory(userID, id uint64) error
	CountMovementsBySubcategory(userID, subcategoryID uint64) (int64, error)
	ReassignSubcategoryMovements(userID, fromID, toID uint64) error

	// Outbound + métricas: lo que un finish de movimiento toca de Telegram y
	// del borde (intent_events, nudges, borrado físico) y que flow no quiere
	// conocer. SendText y StartFlow son los que vuelven a flow como salida.
	ResolveMetric(ctx context.Context, userID uint64, outcome string, movementIDs ...uint)
	SendText(ctx context.Context, b *bot.Bot, chatID int64, text string)
	StartFlow(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, flowName string, seed conversation.Data, errCtx string) error
	MarkTipSent(userID uint64, tip string) error
	SoftDeleteByIDs(ids []uint) error
	StartAccountCreate(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error
	SuggestMergeTarget(ctx context.Context, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory
}
