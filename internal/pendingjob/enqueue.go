package pendingjob

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

const (
	KindFreeText   = "free_text"
	KindUpdatePick = "update_pick"
)

type FreeTextPayload struct {
	Text string `json:"text"`
}

type UpdatePickPayload struct {
	Message       string                 `json:"message"`
	TransactionID string                 `json:"transaction_id"`
	OldIDs        []string               `json:"old_ids"`
	BeforeRows    []movement.MovementRow `json:"before_rows"`
}

type replayingKey struct{}

func WithReplaying(ctx context.Context) context.Context {
	return context.WithValue(ctx, replayingKey{}, true)
}
func IsReplaying(ctx context.Context) bool {
	v, _ := ctx.Value(replayingKey{}).(bool)
	return v
}

func bumpNextDrainAt(t time.Time) {
	drainMu.Lock()
	defer drainMu.Unlock()
	if t.After(nextDrainAt) {
		nextDrainAt = t
	}
}

func HandleGroqError(ctx context.Context, s Services, repo Repository, chat messenger.Chat, userID uint64, text string, err error) (bool, error) {
	if EnqueueFreeText(ctx, s, repo, chat, userID, text, err) {
		return true, nil
	}
	var rl *orchestrator.RateLimitedError
	if errors.As(err, &rl) {
		return true, err
	}
	return false, nil
}

func EnqueueFreeText(ctx context.Context, s Services, repo Repository, chat messenger.Chat, userID uint64, text string, err error) bool {
	var rl *orchestrator.RateLimitedError
	if !errors.As(err, &rl) || IsReplaying(ctx) {
		return false
	}
	payload, _ := json.Marshal(FreeTextPayload{Text: text})
	if ierr := repo.Insert(&PendingJob{UserID: userID, Kind: KindFreeText, Payload: payload}); ierr != nil {
		slog.ErrorContext(ctx, "enqueue free_text failed", "user_id", userID, "err", ierr)
		s.SendText(ctx, chat, msgCouldNotSave("tu mensaje"))
		return true
	}
	bumpNextDrainAt(time.Now().Add(rl.RetryAfter))
	s.SendText(ctx, chat, AckForWait(rl.RetryAfter))
	return true
}

func EnqueueUpdatePick(ctx context.Context, s Services, repo Repository, chat messenger.Chat, userID uint64, message, txID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool {
	var rl *orchestrator.RateLimitedError
	if !errors.As(err, &rl) {
		return false
	}
	payload, _ := json.Marshal(UpdatePickPayload{Message: message, TransactionID: txID, OldIDs: oldIDs, BeforeRows: beforeRows})
	if ierr := repo.Insert(&PendingJob{UserID: userID, Kind: KindUpdatePick, Payload: payload}); ierr != nil {
		slog.ErrorContext(ctx, "enqueue update_pick failed", "user_id", userID, "err", ierr)
		s.SendText(ctx, chat, msgCouldNotSave("el cambio"))
		return true
	}
	bumpNextDrainAt(time.Now().Add(rl.RetryAfter))
	s.SendText(ctx, chat, AckForWait(rl.RetryAfter))
	return true
}

func EnqueueBehindPending(ctx context.Context, s Services, repo Repository, chat messenger.Chat, userID uint64, text string) bool {
	n, err := repo.CountByUser(userID)
	if err != nil {
		slog.ErrorContext(ctx, "enqueue behind pending: count failed, processing live", "user_id", userID, "err", err)
		return false
	}
	if n == 0 {
		return false
	}
	payload, _ := json.Marshal(FreeTextPayload{Text: text})
	if ierr := repo.Insert(&PendingJob{UserID: userID, Kind: KindFreeText, Payload: payload}); ierr != nil {
		slog.ErrorContext(ctx, "enqueue behind pending failed", "user_id", userID, "err", ierr)
		return false
	}
	s.SendText(ctx, chat, msgQueuedBehindPending)
	return true
}
