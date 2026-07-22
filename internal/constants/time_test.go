package constants

import (
	"testing"
	"time"
)

func TestArgentinaZone_IsUTCMinus3(t *testing.T) {
	// 01:30 UTC del 1 de agosto sigue siendo 31 de julio en Argentina. Es el
	// caso que rompe un cursor de mes calculado en UTC.
	utc := time.Date(2026, 8, 1, 1, 30, 0, 0, time.UTC)
	local := utc.In(ArgentinaZone)
	if local.Month() != time.July || local.Day() != 31 {
		t.Fatalf("got %s, want 31 de julio", local.Format("2006-01-02 15:04"))
	}
}
