package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingjob"
)

const (
	kindFreeText   = "free_text"
	kindUpdatePick = "update_pick"

	// ackShortWaitThreshold: por debajo, el wait es TPM (segundos) → ack corto;
	// por encima, TPD (raro) → ack con ETA. Solo cambia la copy, nunca hay silencio.
	ackShortWaitThreshold = 60 * time.Second
)

type freeTextPayload struct {
	Text string `json:"text"`
}

type updatePickPayload struct {
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

func withReplaying(ctx context.Context) context.Context {
	return context.WithValue(ctx, replayingKey{}, true)
}
func isReplaying(ctx context.Context) bool {
	v, _ := ctx.Value(replayingKey{}).(bool)
	return v
}

// bumpNextDrainAt sube nextDrainAt al máximo visto (org-wide, in-memory).
func (c *controller) bumpNextDrainAt(t time.Time) {
	c.drainMu.Lock()
	defer c.drainMu.Unlock()
	if t.After(c.nextDrainAt) {
		c.nextDrainAt = t
	}
}

// handleGroqError decide qué hacer con un error de una llamada Groq en un site
// del webhook que replaya como free_text. Devuelve (handled, outErr): si handled,
// el caller hace `return outErr` sin mandar su copy genérica.
//   - webhook + RateLimited → encola free_text + ackea → (true, nil)
//   - replay  + RateLimited → propaga el error al drain, sin mensaje → (true, err)
//   - cualquier no-429 → (false, nil): el caller manda su copy de error de siempre
func (c *controller) handleGroqError(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string, err error) (bool, error) {
	if c.enqueueIfRateLimited(ctx, b, chatID, userID, text, err) {
		return true, nil
	}
	var rl *orchestrator.RateLimitedError
	if errors.As(err, &rl) {
		return true, err // replay: el drain gatea y deja el job
	}
	return false, nil
}

// enqueueIfRateLimited encola un free_text y ackea si err es un 429. No hace nada
// (false) si no es 429 o si estamos en replay (el drain maneja el 429).
func (c *controller) enqueueIfRateLimited(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string, err error) bool {
	var rl *orchestrator.RateLimitedError
	if !errors.As(err, &rl) || isReplaying(ctx) {
		return false
	}
	payload, _ := json.Marshal(freeTextPayload{Text: text})
	if ierr := c.jobs.Insert(&pendingjob.PendingJob{UserID: userID, Kind: kindFreeText, Payload: payload}); ierr != nil {
		// El enqueue falló: el mensaje del usuario se perdió de verdad → tiene que
		// saberlo (msgCouldNotSave, no msgSomethingBroke).
		slog.ErrorContext(ctx, "enqueue free_text failed", "user_id", userID, "err", ierr)
		c.sendText(ctx, b, chatID, msgCouldNotSave("tu mensaje"))
		return true
	}
	c.bumpNextDrainAt(time.Now().Add(rl.RetryAfter))
	c.sendText(ctx, b, chatID, ackForWait(rl.RetryAfter))
	return true
}

// enqueueUpdatePickIfRateLimited encola un update_pick (preserva el pick del
// usuario). Solo lo llama finishMovementUpdatePickFlow (webhook-only), así que no
// necesita el guard de replay.
func (c *controller) enqueueUpdatePickIfRateLimited(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, message, txID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool {
	var rl *orchestrator.RateLimitedError
	if !errors.As(err, &rl) {
		return false
	}
	payload, _ := json.Marshal(updatePickPayload{Message: message, TransactionID: txID, OldIDs: oldIDs, BeforeRows: beforeRows})
	if ierr := c.jobs.Insert(&pendingjob.PendingJob{UserID: userID, Kind: kindUpdatePick, Payload: payload}); ierr != nil {
		slog.ErrorContext(ctx, "enqueue update_pick failed", "user_id", userID, "err", ierr)
		c.sendText(ctx, b, chatID, msgCouldNotSave("el cambio"))
		return true
	}
	c.bumpNextDrainAt(time.Now().Add(rl.RetryAfter))
	c.sendText(ctx, b, chatID, ackForWait(rl.RetryAfter))
	return true
}

// ackForWait ramifica la copy según la magnitud del wait. Nunca silencioso.
func ackForWait(d time.Duration) string {
	if d <= ackShortWaitThreshold {
		return msgAckShortWait
	}
	mins := int(d.Round(time.Minute).Minutes())
	if mins < 1 {
		mins = 1
	}
	return fmt.Sprintf(msgAckLongWaitFmt, mins)
}

// enqueueBehindPending: si el usuario ya tiene jobs pendientes, encola este texto
// también y ackea — aunque el cupo haya vuelto — para que el drain lo procese en
// orden (FIFO). Evita que "no, 600" se procese antes de "gasté 500". Solo texto
// libre, solo en el webhook (el drain no pasa por acá). Devuelve true si encoló.
func (c *controller) enqueueBehindPending(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) bool {
	n, err := c.jobs.CountByUser(userID)
	if err != nil || n == 0 {
		return false
	}
	payload, _ := json.Marshal(freeTextPayload{Text: text})
	if ierr := c.jobs.Insert(&pendingjob.PendingJob{UserID: userID, Kind: kindFreeText, Payload: payload}); ierr != nil {
		slog.ErrorContext(ctx, "enqueue behind pending failed", "user_id", userID, "err", ierr)
		return false // no pudimos encolar → dejá que el flujo normal intente
	}
	c.sendText(ctx, b, chatID, msgQueuedBehindPending)
	return true
}
