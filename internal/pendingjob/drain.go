package pendingjob

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/user"
)

const (
	// JobDrainInterval: el gating (nextDrainAt) hace barato tickear seguido;
	// liberado el cupo, drena en ≤30s. Sin config nuevo.
	JobDrainInterval = 30 * time.Second
	// MaxJobAge: más allá de esto un job no es rate-limit sino falla permanente
	// (key muerta, billing, provider caído). > el reset más grande que honramos
	// (TPD rolling, ~16 min observado); 2h da ~7× margen.
	MaxJobAge = 2 * time.Hour
)

var (
	drainMu     sync.Mutex
	nextDrainAt time.Time
)

// Run tickea cada interval hasta que ctx se cancela (patrón Sweeper.Run).
func Run(ctx context.Context, s Services, repo Repository, b *bot.Bot, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			drainTick(ctx, s, repo, b, time.Now())
		}
	}
}

func drainTick(ctx context.Context, s Services, repo Repository, b *bot.Bot, now time.Time) {
	drainMu.Lock()
	gated := now.Before(nextDrainAt)
	drainMu.Unlock()
	if gated {
		return // sin cupo: no probes inútiles
	}
	userIDs, err := repo.ListPendingUserIDs()
	if err != nil {
		slog.ErrorContext(ctx, "drain: list users failed", "err", err)
		return
	}
	for _, userID := range userIDs {
		drainUser(ctx, s, repo, b, userID, now)
	}
}

func drainUser(ctx context.Context, s Services, repo Repository, b *bot.Bot, userID uint64, now time.Time) {
	u, err := s.UsersFindByID(userID)
	if err != nil {
		slog.ErrorContext(ctx, "drain: user lookup failed", "user_id", userID, "err", err)
		return
	}
	channelID, err := s.UsersFindChannelID(u.ID, user.ChannelTelegram)
	if err != nil {
		slog.ErrorContext(ctx, "drain: channel lookup failed", "user_id", userID, "err", err)
		return
	}
	chatID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		slog.ErrorContext(ctx, "drain: bad telegram_id", "user_id", userID, "err", err)
		return
	}
	jobs, err := repo.ListByUserOrdered(userID)
	if err != nil {
		slog.ErrorContext(ctx, "drain: list jobs failed", "user_id", userID, "err", err)
		return
	}
	for _, job := range jobs {
		// Give-up ANTES del replay: no gastar una llamada en un job que abandonamos.
		// FIFO (oldest-first) → el primer job vivo marca el corte.
		// ponytail: corre tras nextDrainAt; una key muerta 429ea sin reset válido →
		// backoff corto → drain sigue tickeando → la edad dispara igual.
		if now.Sub(job.CreatedAt) > MaxJobAge {
			slog.WarnContext(ctx, "drain: job gave up", "event", "job_gave_up", "user_id", userID, "kind", job.Kind, "age", now.Sub(job.CreatedAt).String())
			s.SendText(ctx, b, chatID, msgJobGaveUp(job))
			_ = repo.Delete(job.ID)
			continue
		}
		// Cada job replayado es su propia unidad de trabajo, así que lleva su
		// propio trace_id y su fila en request_traces. El ctx del drenaje viene
		// del ticker del server y no trae ninguno: sin esto, las llamadas a
		// Groq del replay escriben con trace_id vacío y el mensaje se pierde de
		// las tres capas. Ver traced() en trace.go.
		var err error
		s.Traced(ctx, updateTypeReplay, "", func(tctx context.Context) (*uint64, error) {
			err = replayJob(WithReplaying(tctx), s, b, chatID, userID, job)
			return &userID, err
		})
		var rl *orchestrator.RateLimitedError
		if errors.As(err, &rl) {
			bumpNextDrainAt(time.Now().Add(rl.RetryAfter))
			slog.InfoContext(ctx, "drain: deferred (429)", "event", "job_deferred", "user_id", userID, "kind", job.Kind)
			return // el drain ES la sonda: cortar el ciclo de este usuario
		}
		if err != nil {
			slog.ErrorContext(ctx, "drain: replay failed (non-429)", "event", "job_drained", "user_id", userID, "kind", job.Kind, "err", err)
		} else {
			slog.InfoContext(ctx, "drain: ok", "event", "job_drained", "user_id", userID, "kind", job.Kind)
		}
		_ = repo.Delete(job.ID)
	}
}

// updateTypeReplay es el update_type de un job drenado. Se distingue de text /
// callback / command a propósito: sin eso, un replay se lee en request_traces
// como si el usuario hubiera escrito de nuevo, y la latencia de la cola queda
// mezclada con la de los mensajes reales.
const updateTypeReplay = "replay"

// replayJob recibe el ctx YA marcado como replay (ver el call site en drainUser):
// el flag se pone una sola vez, afuera, y no en cada case — un case nuevo que se
// olvidara de marcarlo volvería a encolar el job que está drenando, en loop.
func replayJob(ctx context.Context, s Services, b *bot.Bot, chatID int64, userID uint64, job PendingJob) error {
	switch job.Kind {
	case KindFreeText:
		var p FreeTextPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return nil // payload corrupto → no-429 → Delete. ponytail: nunca bloquea la cola.
		}
		return s.HandleFreeText(ctx, b, chatID, userID, p.Text)
	case KindUpdatePick:
		var p UpdatePickPayload
		if err := json.Unmarshal(job.Payload, &p); err != nil {
			return nil
		}
		return s.ProceedToUpdateConfirm(ctx, b, chatID, userID, p.Message, p.TransactionID, p.OldIDs, p.BeforeRows)
	default:
		slog.WarnContext(ctx, "drain: unknown kind", "kind", job.Kind)
		return nil
	}
}

// msgJobGaveUp: el drain se rindió con un job (429 permanente). Reusa
// msgCouldNotSave — el caso es exactamente el suyo ("no se guardó nada") y así
// hereda el tono de los 5 mensajes por-significado en vez de inventar copy
// paralela. Para free_text echoa el texto para que el usuario copie y pegue.
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
