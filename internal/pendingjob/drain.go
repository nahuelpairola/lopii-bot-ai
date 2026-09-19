package pendingjob

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/orchestrator"
)

type chatResolver interface {
	ChatFor(userID uint64) (messenger.Chat, error)
}

const (
	JobDrainInterval = 30 * time.Second
	MaxJobAge        = 2 * time.Hour
)

var (
	drainMu     sync.Mutex
	nextDrainAt time.Time
)

func Run(ctx context.Context, s Services, repo Repository, chats chatResolver, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			drainTick(ctx, s, repo, chats, time.Now())
		}
	}
}

func drainTick(ctx context.Context, s Services, repo Repository, chats chatResolver, now time.Time) {
	drainMu.Lock()
	gated := now.Before(nextDrainAt)
	drainMu.Unlock()
	if gated {
		return
	}
	userIDs, err := repo.ListPendingUserIDs()
	if err != nil {
		slog.ErrorContext(ctx, "drain: list users failed", "err", err)
		return
	}
	for _, userID := range userIDs {
		drainUser(ctx, s, repo, chats, userID, now)
	}
}

func drainUser(ctx context.Context, s Services, repo Repository, chats chatResolver, userID uint64, now time.Time) {
	u, err := s.UsersFindByID(userID)
	if err != nil {
		slog.ErrorContext(ctx, "drain: user lookup failed", "user_id", userID, "err", err)
		return
	}
	chat, err := chats.ChatFor(u.ID)
	if err != nil {
		slog.ErrorContext(ctx, "drain: chat lookup failed", "user_id", userID, "err", err)
		return
	}
	jobs, err := repo.ListByUserOrdered(userID)
	if err != nil {
		slog.ErrorContext(ctx, "drain: list jobs failed", "user_id", userID, "err", err)
		return
	}
	for _, job := range jobs {
		if now.Sub(job.CreatedAt) > MaxJobAge {
			slog.WarnContext(ctx, "drain: job gave up", "event", "job_gave_up", "user_id", userID, "kind", job.Kind, "age", now.Sub(job.CreatedAt).String())
			s.SendText(ctx, chat, msgJobGaveUp(job))
			_, _ = repo.Delete(job.ID)
			continue
		}
		var err error
		s.Traced(ctx, updateTypeReplay, "", func(tctx context.Context) (*uint64, error) {
			err = replayJob(WithReplaying(tctx), s, chat, userID, job)
			return &userID, err
		})
		var rl *orchestrator.RateLimitedError
		if errors.As(err, &rl) {
			bumpNextDrainAt(time.Now().Add(rl.RetryAfter))
			slog.InfoContext(ctx, "drain: deferred (429)", "event", "job_deferred", "user_id", userID, "kind", job.Kind)
			return
		}
		if err != nil {
			slog.ErrorContext(ctx, "drain: replay failed (non-429)", "event", "job_drained", "user_id", userID, "kind", job.Kind, "err", err)
		} else {
			slog.InfoContext(ctx, "drain: ok", "event", "job_drained", "user_id", userID, "kind", job.Kind)
		}
		_, _ = repo.Delete(job.ID)
	}
}

const updateTypeReplay = "replay"

func replayJob(ctx context.Context, s Services, chat messenger.Chat, userID uint64, job PendingJob) error {
	switch job.Kind {
	case KindFreeText:
		var p FreeTextPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return nil
		}
		return s.HandleFreeText(ctx, chat, userID, p.Text)
	case KindUpdatePick:
		var p UpdatePickPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return nil
		}
		return s.ProceedToUpdateConfirm(ctx, chat, userID, p.Message, p.TransactionID, p.OldIDs, p.BeforeRows)
	default:
		slog.WarnContext(ctx, "drain: unknown kind", "kind", job.Kind)
		return nil
	}
}

func msgJobGaveUp(job PendingJob) string {
	if job.Kind != KindFreeText {
		return msgCouldNotSave("el cambio")
	}
	var p FreeTextPayload
	_ = json.Unmarshal(job.Payload, &p)
	txt := p.Text
	if len([]rune(txt)) > 40 {
		txt = string([]rune(txt)[:40]) + "…"
	}
	return msgCouldNotSave("«" + txt + "»")
}
