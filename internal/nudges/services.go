// Package nudges es el dispatcher de tips contextuales, extraído de
// controller/messaging. Conoce los gates, los cooldowns y las keys, y delega
// en la interfaz Services todo lo que necesita del mundo: repos de lectura,
// el engine de conversaciones, los outbounds a Telegram y el loop de QUERY.
// Vive en su propio paquete y NO sabe nada de Telegram-webhook ni del
// controller — el borde lo implementa con puentes de una línea.
package nudges

import (
	"context"
	"time"

	"github.com/go-telegram/bot"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

// Services es lo que el dispatcher de tips necesita del mundo, con el
// *controller* como implementación. Métodos exportados porque una interfaz
// con métodos unexported solo la implementan tipos del mismo paquete; el
// controller la implementa con puentes de una línea (nudges_services.go).
type Services interface {
	// El engine de conversaciones: un flow abierto frena los tips.
	EngineInProgress(userID uint64) (bool, error)

	// Repos de lectura.
	RemindersFindByUserID(userID uint64) (*reminder.Reminder, error)
	UsersFindByID(userID uint64) (*user.User, error)
	AccountsFindByUserID(userID uint64) ([]account.Account, error)
	SubcategoriesFindByCategoryAndSubcategory(userID uint64, category, name string) (*subcategory.Subcategory, error)
	MovementsCountBySubcategory(userID uint64, subcategoryID uint64) (int64, error)
	MovementsCountByDayForUser(userID uint64, from, to time.Time) ([]movement.DayCount, error)
	MovementsCountForUser(userID uint64) (int64, error)
	MovementsSumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	MovementsSumAmountForAccount(accountID uint64) (decimal.Decimal, error)

	// El storage once-ever/cooldown de user_nudges (internal/nudge).
	NudgesAvailable() bool
	NudgesSentKeys(userID uint64) ([]string, error)
	NudgesLastSentAt(userID uint64) (*time.Time, error)
	NudgesMarkSent(userID uint64, key string) error
	NudgesMarkSentAgain(userID uint64, key string) error
	NudgesMarkTapped(userID uint64, key string) error

	// Outbounds a Telegram y al loop de QUERY.
	SendText(ctx context.Context, b *bot.Bot, chatID int64, text string)
	SendPrompt(ctx context.Context, b *bot.Bot, chatID int64, prompt conversation.Prompt)
	HandleQuery(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) (bool, error)
}
