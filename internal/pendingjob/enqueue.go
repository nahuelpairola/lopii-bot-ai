package pendingjob

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
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

// replayingKey marca un ctx que corre dentro del drain. Las sites de error-Groq
// lo consultan: en el webhook encolan+ackean el 429; en replay lo propagan al
// drain (que gatea y deja el job) SIN re-encolar ni re-ackear. Sin esto el drain
// re-encolaría el job que está drenando (loop) y spamearía el ack.
type replayingKey struct{}

func WithReplaying(ctx context.Context) context.Context {
	return context.WithValue(ctx, replayingKey{}, true)
}
func IsReplaying(ctx context.Context) bool {
	v, _ := ctx.Value(replayingKey{}).(bool)
	return v
}

// bumpNextDrainAt sube nextDrainAt al máximo visto (org-wide, in-memory).
func bumpNextDrainAt(t time.Time) {
	drainMu.Lock()
	defer drainMu.Unlock()
	if t.After(nextDrainAt) {
		nextDrainAt = t
	}
}

// HandleGroqError decide qué hacer con un error de una llamada Groq en un site
// del webhook que replaya como free_text. Devuelve (handled, outErr): si handled,
// el caller hace `return outErr` sin mandar su copy genérica.
//   - webhook + RateLimited → encola free_text + ackea → (true, nil)
//   - replay  + RateLimited → propaga el error al drain, sin mensaje → (true, err)
//   - cualquier no-429 → (false, nil): el caller manda su copy de error de siempre
func HandleGroqError(ctx context.Context, s Services, repo Repository, b *bot.Bot, chatID int64, userID uint64, text string, err error) (bool, error) {
	if EnqueueFreeText(ctx, s, repo, b, chatID, userID, text, err) {
		return true, nil
	}
	var rl *orchestrator.RateLimitedError
	if errors.As(err, &rl) {
		return true, err // replay: el drain gatea y deja el job
	}
	return false, nil
}

// EnqueueFreeText encola un free_text y ackea si err es un 429. No hace nada
// (false) si no es 429 o si estamos en replay (el drain maneja el 429).
func EnqueueFreeText(ctx context.Context, s Services, repo Repository, b *bot.Bot, chatID int64, userID uint64, text string, err error) bool {
	var rl *orchestrator.RateLimitedError
	if !errors.As(err, &rl) || IsReplaying(ctx) {
		return false
	}
	payload, _ := json.Marshal(FreeTextPayload{Text: text})
	if ierr := repo.Insert(&PendingJob{UserID: userID, Kind: KindFreeText, Payload: payload}); ierr != nil {
		// El enqueue falló: el mensaje del usuario se perdió de verdad → tiene que
		// saberlo (msgCouldNotSave, no msgSomethingBroke).
		slog.ErrorContext(ctx, "enqueue free_text failed", "user_id", userID, "err", ierr)
		s.SendText(ctx, b, chatID, msgCouldNotSave("tu mensaje"))
		return true
	}
	bumpNextDrainAt(time.Now().Add(rl.RetryAfter))
	s.SendText(ctx, b, chatID, AckForWait(rl.RetryAfter))
	return true
}

// EnqueueUpdatePick encola un update_pick (preserva el pick del
// usuario). Solo lo llama finishMovementUpdatePickFlow (webhook-only), así que no
// necesita el guard de replay.
func EnqueueUpdatePick(ctx context.Context, s Services, repo Repository, b *bot.Bot, chatID int64, userID uint64, message, txID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool {
	var rl *orchestrator.RateLimitedError
	if !errors.As(err, &rl) {
		return false
	}
	payload, _ := json.Marshal(UpdatePickPayload{Message: message, TransactionID: txID, OldIDs: oldIDs, BeforeRows: beforeRows})
	if ierr := repo.Insert(&PendingJob{UserID: userID, Kind: KindUpdatePick, Payload: payload}); ierr != nil {
		slog.ErrorContext(ctx, "enqueue update_pick failed", "user_id", userID, "err", ierr)
		s.SendText(ctx, b, chatID, msgCouldNotSave("el cambio"))
		return true
	}
	bumpNextDrainAt(time.Now().Add(rl.RetryAfter))
	s.SendText(ctx, b, chatID, AckForWait(rl.RetryAfter))
	return true
}

// EnqueueBehindPending: si el usuario ya tiene jobs pendientes, encola este texto
// también y ackea — aunque el cupo haya vuelto — para que el drain lo procese en
// orden (FIFO). Evita que "no, 600" se procese antes de "gasté 500". Solo texto
// libre, solo en el webhook (el drain no pasa por acá). Devuelve true si encoló.
func EnqueueBehindPending(ctx context.Context, s Services, repo Repository, b *bot.Bot, chatID int64, userID uint64, text string) bool {
	n, err := repo.CountByUser(userID)
	if err != nil || n == 0 {
		return false
	}
	payload, _ := json.Marshal(FreeTextPayload{Text: text})
	if ierr := repo.Insert(&PendingJob{UserID: userID, Kind: KindFreeText, Payload: payload}); ierr != nil {
		slog.ErrorContext(ctx, "enqueue behind pending failed", "user_id", userID, "err", ierr)
		return false // no pudimos encolar → dejá que el flujo normal intente
	}
	s.SendText(ctx, b, chatID, msgQueuedBehindPending)
	return true
}
