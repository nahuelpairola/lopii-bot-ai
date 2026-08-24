package messaging

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/nudges"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

// Los métodos de este archivo implementan nudges.Services: lo que el
// dispatcher de tips necesita del mundo, con el *controller* como
// implementación. Puentes de una línea, como en agent_services.go.
// *controller implementa la interfaz directamente desde la Task 7 — SendText,
// SendPrompt y HandleQuery ya sólo piden messenger.Chat, así que no hace
// falta un puente.
var _ nudges.Services = (*controller)(nil)

func (c *controller) EngineInProgress(userID uint64) (bool, error) {
	return c.engine.InProgress(userID)
}

func (c *controller) RemindersFindByUserID(userID uint64) (*reminder.Reminder, error) {
	return c.reminders.FindByUserID(userID)
}

func (c *controller) UsersFindByID(userID uint64) (*user.User, error) {
	return c.users.FindByID(userID)
}

func (c *controller) UsersFindChannelID(userID uint64, channel string) (string, error) {
	return c.users.FindChannelID(userID, channel)
}

func (c *controller) AccountsFindByUserID(userID uint64) ([]account.Account, error) {
	return c.accounts.FindByUserID(userID)
}

func (c *controller) SubcategoriesFindByCategoryAndSubcategory(userID uint64, category, name string) (*subcategory.Subcategory, error) {
	return c.subcategories.FindByCategoryAndSubcategory(userID, category, name)
}

func (c *controller) MovementsCountBySubcategory(userID uint64, subcategoryID uint64) (int64, error) {
	return c.movements.CountBySubcategory(userID, subcategoryID)
}

func (c *controller) MovementsCountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error) {
	return c.movements.CountByDayForUser(userID, from, to)
}

func (c *controller) MovementsCountForUser(userID uint64) (int64, error) {
	return c.movements.CountForUser(userID)
}

func (c *controller) MovementsSumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	return c.movements.SumForUser(q, groupBy)
}

func (c *controller) MovementsSumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	return c.movements.SumAmountForAccount(accountID)
}

func (c *controller) NudgesAvailable() bool {
	return c.nudges != nil
}

func (c *controller) NudgesSentKeys(userID uint64) ([]string, error) {
	return c.nudges.SentKeys(userID)
}

func (c *controller) NudgesLastSentAt(userID uint64) (*time.Time, error) {
	return c.nudges.LastSentAt(userID)
}

func (c *controller) NudgesMarkSent(userID uint64, key string) error {
	return c.nudges.MarkSent(userID, key)
}

func (c *controller) NudgesMarkSentAgain(userID uint64, key string) error {
	return c.nudges.MarkSentAgain(userID, key)
}

func (c *controller) NudgesMarkTapped(userID uint64, key string) error {
	return c.nudges.MarkTapped(userID, key)
}

func (c *controller) HandleQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) (bool, error) {
	return c.handleQuery(ctx, chat, userID, text)
}
