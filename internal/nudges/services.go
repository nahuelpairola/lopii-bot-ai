package nudges

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

type Services interface {
	EngineInProgress(userID uint64) (bool, error)

	RemindersFindByUserID(userID uint64) (*reminder.Reminder, error)
	UsersFindByID(userID uint64) (*user.User, error)
	AccountsFindByUserID(userID uint64) ([]account.Account, error)
	SubcategoriesFindByCategoryAndSubcategory(userID uint64, category, name string) (*subcategory.Subcategory, error)
	MovementsCountBySubcategory(userID uint64, subcategoryID uint64) (int64, error)
	MovementsCountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error)
	MovementsCountForUser(userID uint64) (int64, error)
	MovementsSumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	MovementsSumAmountForAccount(accountID uint64) (decimal.Decimal, error)

	NudgesAvailable() bool
	NudgesSentKeys(userID uint64) ([]string, error)
	NudgesLastSentAt(userID uint64) (*time.Time, error)
	NudgesMarkSent(userID uint64, key string) error
	NudgesMarkSentAgain(userID uint64, key string) error
	NudgesMarkTapped(userID uint64, key string) error

	SendText(ctx context.Context, chat messenger.Chat, text string)
	SendPrompt(ctx context.Context, chat messenger.Chat, prompt conversation.Prompt)
	HandleQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) (bool, error)
}
