package messaging

import (
	"testing"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/movement"
)

func TestTodayCivil_IsArgentineCalendarDay(t *testing.T) {
	got := movement.TodayCivil()
	want := time.Now().In(constants.ArgentinaZone).Format("2006-01-02")
	if got.Format("2006-01-02") != want {
		t.Errorf("movement.TodayCivil() = %s, want %s (día ART)", got.Format("2006-01-02"), want)
	}
	if got.Location() != time.UTC || got.Hour() != 0 {
		t.Errorf("movement.TodayCivil() = %v, want medianoche UTC (fecha civil comparable)", got)
	}
}
