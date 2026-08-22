package pendingjob

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/user"
)

// Services is what the drain and enqueue functions need from the world,
// with the *controller as implementation. Bridge file: messaging/pendingjob_services.go.
type Services interface {
	UsersFindByID(userID uint64) (*user.User, error)
	UsersFindChannelID(userID uint64, channel string) (string, error)
	HandleFreeText(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error
	ProceedToUpdateConfirm(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, message string, transactionID string, oldIDs []string, beforeRows []movement.MovementRow) error
	SendText(ctx context.Context, b *bot.Bot, chatID int64, text string)
	Traced(ctx context.Context, updateType string, traceID string, fn func(tctx context.Context) (*uint64, error))
}
