package flow

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
)

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

	FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error)

	UpsertReminder(rem *reminder.Reminder) error
	DisableReminder(userID uint64) error
	SetWeeklySummary(userID uint64, enabled bool) error

	ResolveMetric(ctx context.Context, userID uint64, outcome string, movementIDs ...uint)
	SendText(ctx context.Context, chat messenger.Chat, text string)
	StartFlow(ctx context.Context, chat messenger.Chat, userID uint64, flowName string, seed conversation.Data, errCtx string) error
	MarkTipSent(userID uint64, tip string) error
	SoftDeleteByIDs(ids []uint) error
	StartAccountCreate(ctx context.Context, chat messenger.Chat, userID uint64, text string) error
	SuggestMergeTarget(ctx context.Context, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory
}
