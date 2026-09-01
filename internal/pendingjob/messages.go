package pendingjob

import (
	"fmt"
	"time"

	"lopiibot.com/internal/flow"
)

const (
	msgAckShortWait   = "Dame un segundo, ya te lo cargo 🙌"
	msgAckLongWaitFmt = "Estoy sin cupo por ~%d min 🙏 lo cargo apenas se libere y te aviso."
)

const ackShortWaitThreshold = 60 * time.Second

const msgQueuedBehindPending = "Ese también, ya te los cargo 🙌"

func msgCouldNotSave(cosa string) string {
	return flow.MsgCouldNotSave(cosa)
}

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
