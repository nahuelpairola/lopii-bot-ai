package movement

import (
	"time"

	"lopiibot.com/internal/constants"
)

// RelativeDate rinde una fecha como la diría una persona. Sin año: los
// candidatos salen de una ventana de días, no de meses.
//
// Compara DÍAS CALENDARIO, no instantes. La fecha de un movimiento es una fecha
// civil que entra por time.Parse("2006-01-02"), o sea medianoche UTC, mientras
// que la medianoche argentina son las 03:00 UTC: comparadas como instantes, todo
// lo cargado hoy caía 3 horas antes del corte y salía "ayer" (visto en Telegram).
// Convertir la fecha a ART tampoco sirve — la corre un día para atrás.
func RelativeDate(d time.Time) string {
	day := CivilDay(d)
	today := TodayCivil()
	switch {
	case !day.Before(today):
		return "hoy"
	case !day.Before(today.AddDate(0, 0, -1)):
		return "ayer"
	default:
		return d.Format("02/01")
	}
}

// CivilDay descarta la hora y la zona: deja solo el día del calendario, anclado
// a UTC para que dos fechas se puedan comparar entre sí sin que el huso mueva
// ninguna de las dos.
func CivilDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// TodayCivil es el día de HOY para el usuario, no para el server. El server
// corre en UTC, así que entre las 21:00 y las 24:00 ART time.Now() ya está en
// el día siguiente: un gasto cargado 21:50 se guardaba con la fecha de mañana
// (visto en producción). Todo lo que signifique "hoy" —la fecha por defecto de
// un movimiento, el "Hoy es" de los prompts, el corte de RelativeDate— sale de
// acá y de ningún otro lado.
func TodayCivil() time.Time {
	return CivilDay(time.Now().In(constants.ArgentinaZone))
}
