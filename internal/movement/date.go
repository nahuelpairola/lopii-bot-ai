package movement

import (
	"time"

	"lopiibot.com/internal/constants"
)

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

var weekdayEs = [...]string{"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"}

func WeekdayEs(t time.Time) string {
	return weekdayEs[t.Weekday()]
}

func CivilDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TodayCivil() time.Time {
	return CivilDay(time.Now().In(constants.ArgentinaZone))
}
