package pendingjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
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

const (
	eventJobGaveUp  = "job_gave_up"
	eventJobDrained = "job_drained"
)

var (
	drainMu     sync.Mutex
	nextDrainAt time.Time
)

var errReplayPanicked = errors.New("pendingjob: replay panicked")

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
	jobs, err := repo.ListByUserOrdered(userID)
	if err != nil {
		slog.ErrorContext(ctx, "drain: list jobs failed", "user_id", userID, "err", err)
		return
	}
	chat, err := chatForUser(s, chats, userID)
	if err != nil {
		slog.ErrorContext(ctx, "drain: user or chat lookup failed", "user_id", userID, "err", err)
		dropExpired(ctx, repo, userID, jobs, now)
		return
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		if now.Sub(job.CreatedAt) > MaxJobAge {
			giveUp(ctx, s, repo, chat, userID, job, now)
			continue
		}
		if !drainJob(ctx, s, repo, chat, userID, job) {
			return
		}
	}
}

func chatForUser(s Services, chats chatResolver, userID uint64) (messenger.Chat, error) {
	u, err := s.UsersFindByID(userID)
	if err != nil {
		return nil, fmt.Errorf("user: %w", err)
	}
	return chats.ChatFor(u.ID)
}

func dropExpired(ctx context.Context, repo Repository, userID uint64, jobs []PendingJob, now time.Time) {
	for _, job := range jobs {
		if now.Sub(job.CreatedAt) <= MaxJobAge {
			continue
		}
		if _, err := repo.Delete(job.ID); err != nil {
			slog.ErrorContext(ctx, "drain: drop expired job failed", "user_id", userID, "job_id", job.ID, "err", err)
			continue
		}
		slog.WarnContext(ctx, "drain: job gave up", "event", eventJobGaveUp, "reason", "lookup_failed", "user_id", userID, "kind", job.Kind, "age", now.Sub(job.CreatedAt).String())
	}
}

func giveUp(ctx context.Context, s Services, repo Repository, chat messenger.Chat, userID uint64, job PendingJob, now time.Time) {
	deleted, err := repo.Delete(job.ID)
	if err != nil {
		slog.ErrorContext(ctx, "drain: give up delete failed", "user_id", userID, "job_id", job.ID, "err", err)
		return
	}
	if !deleted {
		return
	}
	slog.WarnContext(ctx, "drain: job gave up", "event", eventJobGaveUp, "reason", "max_age", "user_id", userID, "kind", job.Kind, "age", now.Sub(job.CreatedAt).String())
	s.SendText(ctx, chat, msgJobGaveUp(job))
}

func drainJob(ctx context.Context, s Services, repo Repository, chat messenger.Chat, userID uint64, job PendingJob) bool {
	claim := newReplayClaim(func() (bool, error) { return repo.Delete(job.ID) })
	var err error
	s.Traced(withClaim(context.WithoutCancel(ctx), claim), updateTypeReplay, "", func(tctx context.Context) (*uint64, error) {
		err = replaySafely(WithReplaying(tctx), s, chat, userID, job)
		return &userID, err
	})

	ran, claimed, claimErr := claim.state()
	switch {
	case claimErr != nil:
		slog.ErrorContext(ctx, "drain: claim failed, job stays for the next tick", "user_id", userID, "job_id", job.ID, "err", claimErr)
		return false
	case ran && !claimed:
		slog.InfoContext(ctx, "drain: claimed by another instance", "event", "job_claimed_elsewhere", "user_id", userID, "job_id", job.ID)
		return true
	case errors.Is(err, errReplayPanicked):
		if claimed {
			s.SendText(ctx, chat, msgJobGaveUp(job))
		}
		return false
	}

	var rl *orchestrator.RateLimitedError
	if errors.As(err, &rl) {
		if claimed {
			requeue(ctx, s, repo, chat, userID, job)
		}
		bumpNextDrainAt(time.Now().Add(rl.RetryAfter))
		slog.InfoContext(ctx, "drain: deferred (429)", "event", "job_deferred", "user_id", userID, "kind", job.Kind)
		return false
	}

	if !ran {
		if _, derr := repo.Delete(job.ID); derr != nil {
			slog.ErrorContext(ctx, "drain: delete after replay failed", "user_id", userID, "job_id", job.ID, "err", derr)
		}
	}
	if err != nil {
		slog.ErrorContext(ctx, "drain: replay failed (non-429)", "event", eventJobDrained, "user_id", userID, "kind", job.Kind, "err", err)
	} else {
		slog.InfoContext(ctx, "drain: ok", "event", eventJobDrained, "user_id", userID, "kind", job.Kind)
	}
	return true
}

func requeue(ctx context.Context, s Services, repo Repository, chat messenger.Chat, userID uint64, job PendingJob) {
	back := PendingJob{UserID: job.UserID, Kind: job.Kind, Payload: job.Payload, CreatedAt: job.CreatedAt}
	if err := repo.Insert(&back); err != nil {
		slog.ErrorContext(ctx, "drain: requeue after 429 failed", "user_id", userID, "job_id", job.ID, "err", err)
		s.SendText(ctx, chat, msgJobGaveUp(job))
	}
}

func replaySafely(ctx context.Context, s Services, chat messenger.Chat, userID uint64, job PendingJob) (err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(ctx, "drain: replay panicked", "event", "job_panicked", "user_id", userID, "kind", job.Kind, "panic", r, "stack", string(debug.Stack()))
			err = errReplayPanicked
		}
	}()
	return replayJob(ctx, s, chat, userID, job)
}

const updateTypeReplay = "replay"

func replayJob(ctx context.Context, s Services, chat messenger.Chat, userID uint64, job PendingJob) error {
	switch job.Kind {
	case KindFreeText:
		var p FreeTextPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("decode %s payload: %w", job.Kind, err)
		}
		return s.HandleFreeText(ctx, chat, userID, p.Text)
	case KindUpdatePick:
		var p UpdatePickPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return fmt.Errorf("decode %s payload: %w", job.Kind, err)
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
