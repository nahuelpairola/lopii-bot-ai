package nudges

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

type flowState interface {
	InProgress(userID uint64) (bool, error)
}

type reminderFinder interface {
	FindByUserID(userID uint64) (*reminder.Reminder, error)
}

type userFinder interface {
	FindByID(id uint64) (*user.User, error)
}

type accountLister interface {
	FindByUserID(userID uint64) ([]account.Account, error)
}

type subcategoryFinder interface {
	FindByCategoryAndSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
}

type movementStats interface {
	CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error)
	CountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error)
	CountForUser(userID uint64) (int64, error)
	SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
}

type sentHistory interface {
	SentKeys(userID uint64) ([]string, error)
	LastSentAt(userID uint64) (*time.Time, error)
	MarkSent(userID uint64, key string) error
	MarkSentAgain(userID uint64, key string) error
	MarkTapped(userID uint64, key string) error
}

type Tips struct {
	engine        flowState
	reminders     reminderFinder
	users         userFinder
	accounts      accountLister
	subcategories subcategoryFinder
	movements     movementStats
	history       sentHistory
	handleQuery   func(ctx context.Context, chat messenger.Chat, userID uint64, text string) (bool, error)
}

func New(
	engine flowState,
	reminders reminderFinder,
	users userFinder,
	accounts accountLister,
	subcategories subcategoryFinder,
	movements movementStats,
	history sentHistory,
	handleQuery func(ctx context.Context, chat messenger.Chat, userID uint64, text string) (bool, error),
) *Tips {
	return &Tips{
		engine:        engine,
		reminders:     reminders,
		users:         users,
		accounts:      accounts,
		subcategories: subcategories,
		movements:     movements,
		history:       history,
		handleQuery:   handleQuery,
	}
}
