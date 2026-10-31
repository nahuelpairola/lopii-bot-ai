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

type accountStore interface {
	FindByUserID(userID uint64) ([]account.Account, error)
	GetAccount(id uint64) (*account.Account, error)
	FindDefaultByCurrency(userID uint64, cur currency.Currency) (*account.Account, error)
	Insert(a *account.Account) error
	Rename(accountID uint64, name string) error
	UnsetDefault(userID uint64, cur currency.Currency) error
	SetDefault(accountID uint64) error
}

type movementStore interface {
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
	FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error)
	CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error)
	InsertBatch(movs []movement.Movement) error
	ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error
	SoftDeleteByIDs(ids []uint) error
	ReassignAccount(fromID, toID uint64) error
	ReassignSubcategory(userID uint64, fromID uint64, toID uint64) error
}

type subcategoryStore interface {
	FindByCategoryAndSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
	IconForCategory(userID uint64, category string) string
	Insert(s *subcategory.Subcategory) error
	Delete(userID uint64, id uint64) error
	Reload() error
}

type reminderStore interface {
	Upsert(r *reminder.Reminder) error
	Disable(userID uint64) error
	SetWeeklySummary(userID uint64, enabled bool) error
}

type tipMarker interface {
	MarkSent(userID uint64, key string) error
}

type Finisher struct {
	accounts           accountStore
	movements          movementStore
	subcategories      subcategoryStore
	reminders          reminderStore
	tips               tipMarker
	engine             *conversation.Engine
	resolveMetric      func(ctx context.Context, userID uint64, outcome string, movementIDs ...uint)
	startAccountCreate func(ctx context.Context, chat messenger.Chat, userID uint64, text string) error
	suggestMergeTarget func(ctx context.Context, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory
}

func New(
	accounts accountStore,
	movements movementStore,
	subcategories subcategoryStore,
	reminders reminderStore,
	tips tipMarker,
	engine *conversation.Engine,
	resolveMetric func(ctx context.Context, userID uint64, outcome string, movementIDs ...uint),
	startAccountCreate func(ctx context.Context, chat messenger.Chat, userID uint64, text string) error,
	suggestMergeTarget func(ctx context.Context, userID, sourceID uint64, data conversation.Data) *subcategory.Subcategory,
) *Finisher {
	return &Finisher{
		accounts:           accounts,
		movements:          movements,
		subcategories:      subcategories,
		reminders:          reminders,
		tips:               tips,
		engine:             engine,
		resolveMetric:      resolveMetric,
		startAccountCreate: startAccountCreate,
		suggestMergeTarget: suggestMergeTarget,
	}
}

func (f *Finisher) markTipSent(userID uint64, tip string) error {
	if f.tips == nil {
		return nil
	}
	return f.tips.MarkSent(userID, tip)
}
