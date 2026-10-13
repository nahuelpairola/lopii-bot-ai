package agent

import "testing"

func TestDateBackedByMessage(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		date string
		want bool
	}{
		{"day and month name", "Elimina hbo max del 3 de octubre", "2026-10-03", true},
		{"leading zero in the day", "el débito de tarjeta del 04 de agosto eran 131306,49", "2026-08-04", true},
		{"numeric day/month", "borrá el del 12/07", "2026-07-12", true},
		{"numeric with year", "borrá el del 3/10/2026", "2026-10-03", true},
		{"abbreviated month", "borrá la nafta del 3 oct", "2026-10-03", true},
		{"both ends of a range", "los gastos del 3 al 5 de agosto", "2026-08-05", true},
		{"weekday the model turned into a date", "Eliminá el helado del sábado pasado", "2026-08-12", false},
		{"el lunes", "La compra de locro del lunes ponela en salidas restaurante", "2026-08-17", false},
		{"right month, wrong day", "borrá hbo max de octubre", "2026-10-03", false},
		{"right day, wrong month", "borrá el del 3 de septiembre", "2026-10-03", false},
		{"day only inside an amount", "borrá los 13140 de panadería de octubre", "2026-10-03", false},
		{"unparseable date", "del 3 de octubre", "ayer", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dateBackedByMessage(tc.msg, tc.date); got != tc.want {
				t.Errorf("dateBackedByMessage(%q, %q) = %v, want %v", tc.msg, tc.date, got, tc.want)
			}
		})
	}
}
