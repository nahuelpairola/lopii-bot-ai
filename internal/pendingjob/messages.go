package pendingjob

import (
	"fmt"
	"time"

	"lopiibot.com/internal/flow"
)

// Ack de la cola de pending jobs (429 terminal de Groq). Nunca silencioso:
// ackShortWaitThreshold decide cuál de las dos rinde.
const (
	msgAckShortWait   = "Dame un segundo, ya te lo cargo 🙌"
	msgAckLongWaitFmt = "Estoy sin cupo por ~%d min 🙏 lo cargo apenas se libere y te aviso."
)

// ackShortWaitThreshold: por debajo, el wait es TPM (segundos) → ack corto;
// por encima, TPD (raro) → ack con ETA. Solo cambia la copy, nunca hay silencio.
const ackShortWaitThreshold = 60 * time.Second

// msgQueuedBehindPending: distinto del ack del 429 (no repetir), plural implica
// que ambos van juntos; sin jerga de cola/pendiente.
const msgQueuedBehindPending = "Ese también, ya te los cargo 🙌"

// msgCouldNotSave vive en flow (MsgCouldNotSave); el alias conserva el nombre
// corto para los callers que aún no se migran.
func msgCouldNotSave(cosa string) string {
	return flow.MsgCouldNotSave(cosa)
}

// AckForWait ramifica la copy según la magnitud del wait. Nunca silencioso.
func AckForWait(d time.Duration) string {
	if d <= ackShortWaitThreshold {
		return msgAckShortWait
	}
	mins := int(d.Round(time.Minute).Minutes())
	if mins < 1 {
		mins = 1
	}
	return fmt.Sprintf(msgAckLongWaitFmt, mins)
}
