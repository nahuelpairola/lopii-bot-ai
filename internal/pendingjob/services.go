package pendingjob

import (
	"context"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/user"
)

type Replayer interface {
	HandleFreeText(ctx context.Context, chat messenger.Chat, userID uint64, text string) error
	ProceedToUpdateConfirm(ctx context.Context, chat messenger.Chat, userID uint64, message string, transactionID string, oldIDs []string, beforeRows []movement.MovementRow) error
	Traced(ctx context.Context, updateType string, traceID string, fn func(tctx context.Context) (*uint64, error))
}

type userFinder interface {
	FindByID(id uint64) (*user.User, error)
}
