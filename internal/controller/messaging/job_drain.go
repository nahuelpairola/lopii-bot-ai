package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingjob"
)

const (
	// jobDrainInterval: el gating (nextDrainAt) hace barato tickear seguido;
	// liberado el cupo, drena en ≤30s. Sin config nuevo.
	jobDrainInterval = 30 * time.Second
	// maxJobAge: más allá de esto un job no es rate-limit sino falla permanente
	// (key muerta, billing, provider caído). > el reset más grande que honramos
	// (TPD rolling, ~16 min observado); 2h da ~7× margen.
	maxJobAge = 2 * time.Hour
)

// JobDrainInterval es el intervalo del worker, expuesto para el arranque en server.
const JobDrainInterval = jobDrainInterval

// RunJobDrain tickea cada interval hasta que ctx se cancela (patrón Sweeper.Run).
func (c *controller) RunJobDrain(ctx context.Context, b *bot.Bot, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.drainTick(ctx, b, time.Now())
		}
	}
}

func (c *controller) drainTick(ctx context.Context, b *bot.Bot, now time.Time) {
	c.drainMu.Lock()
	gated := now.Before(c.nextDrainAt)
	c.drainMu.Unlock()
	if gated {
		return // sin cupo: no probes inútiles
	}
	userIDs, err := c.jobs.ListPendingUserIDs()
	if err != nil {
		slog.ErrorContext(ctx, "drain: list users failed", "err", err)
		return
	}
	for _, userID := range userIDs {
		c.drainUser(ctx, b, userID, now)
	}
}

func (c *controller) drainUser(ctx context.Context, b *bot.Bot, userID uint64, now time.Time) {
	u, err := c.users.FindByID(userID)
	if err != nil {
		slog.ErrorContext(ctx, "drain: user lookup failed", "user_id", userID, "err", err)
		return
	}
	chatID, err := strconv.ParseInt(u.TelegramID, 10, 64)
	if err != nil {
		slog.ErrorContext(ctx, "drain: bad telegram_id", "user_id", userID, "err", err)
		return
	}
	jobs, err := c.jobs.ListByUserOrdered(userID)
	if err != nil {
		slog.ErrorContext(ctx, "drain: list jobs failed", "user_id", userID, "err", err)
		return
	}
	for _, job := range jobs {
		// Give-up ANTES del replay: no gastar una llamada en un job que abandonamos.
		// FIFO (oldest-first) → el primer job vivo marca el corte.
		// ponytail: corre tras nextDrainAt; una key muerta 429ea sin reset válido →
		// backoff corto → drain sigue tickeando → la edad dispara igual.
		if now.Sub(job.CreatedAt) > maxJobAge {
			slog.WarnContext(ctx, "drain: job gave up", "event", "job_gave_up", "user_id", userID, "kind", job.Kind, "age", now.Sub(job.CreatedAt).String())
			c.sendText(ctx, b, chatID, msgJobGaveUp(job))
			_ = c.jobs.Delete(job.ID)
			continue
		}
		err := c.replayJob(withReplaying(ctx), b, chatID, userID, job)
		var rl *orchestrator.RateLimitedError
		if errors.As(err, &rl) {
			c.bumpNextDrainAt(time.Now().Add(rl.RetryAfter))
			slog.InfoContext(ctx, "drain: deferred (429)", "event", "job_deferred", "user_id", userID, "kind", job.Kind)
			return // el drain ES la sonda: cortar el ciclo de este usuario
		}
		if err != nil {
			slog.ErrorContext(ctx, "drain: replay failed (non-429)", "event", "job_drained", "user_id", userID, "kind", job.Kind, "err", err)
		} else {
			slog.InfoContext(ctx, "drain: ok", "event", "job_drained", "user_id", userID, "kind", job.Kind)
		}
		_ = c.jobs.Delete(job.ID)
	}
}

func (c *controller) replayJob(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, job pendingjob.PendingJob) error {
	switch job.Kind {
	case kindFreeText:
		var p freeTextPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return nil // payload corrupto → no-429 → Delete. ponytail: nunca bloquea la cola.
		}
		return c.handleFreeText(ctx, b, chatID, userID, p.Text)
	case kindUpdatePick:
		var p updatePickPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return nil
		}
		return c.proceedToUpdateConfirm(ctx, b, chatID, userID, p.Message, p.TransactionID, p.OldIDs, p.BeforeRows)
	default:
		slog.WarnContext(ctx, "drain: unknown kind", "kind", job.Kind)
		return nil
	}
}
