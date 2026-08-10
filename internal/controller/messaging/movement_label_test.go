package messaging

import (
	"testing"
	"time"

	"lopiibot.com/internal/constants"
)

// TestTodayCivil_IsArgentineCalendarDay falla si "hoy" vuelve a salir del reloj
// del server: entre las 21:00 y las 24:00 ART, UTC ya está en el día siguiente.
func TestTodayCivil_IsArgentineCalendarDay(t *testing.T) {
	got := todayCivil()
	want := time.Now().In(constants.ArgentinaZone).Format("2006-01-02")
	if got.Format("2006-01-02") != want {
		t.Errorf("todayCivil() = %s, want %s (día ART)", got.Format("2006-01-02"), want)
	}
	if got.Location() != time.UTC || got.Hour() != 0 {
		t.Errorf("todayCivil() = %v, want medianoche UTC (fecha civil comparable)", got)
	}
}
