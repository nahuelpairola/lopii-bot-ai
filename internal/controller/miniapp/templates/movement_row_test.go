package templates

import (
	"testing"
	"time"
)

func TestRowDate(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{"día normal", time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC), "12 ago"},
		// El caso que importa: la fecha de un movimiento es civil, guardada como
		// medianoche UTC. Convertirla a ART la correría al 31 de julio.
		{"primero de mes no se cae al mes anterior", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), "1 ago"},
		{"enero", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), "5 ene"},
		{"diciembre", time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC), "31 dic"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RowDate(c.in); got != c.want {
				t.Errorf("RowDate = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRowTitle(t *testing.T) {
	desc := "Coto"
	blank := "   "
	cases := []struct {
		name        string
		description *string
		subcategory string
		want        string
	}{
		{"usa la descripción", &desc, "Súper", "Coto"},
		{"sin descripción cae a la subcategoría", nil, "Súper", "Súper"},
		{"descripción en blanco cae a la subcategoría", &blank, "Súper", "Súper"},
		{"sin nada no rompe", nil, "", RowFallbackTitle},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RowTitle(c.description, c.subcategory); got != c.want {
				t.Errorf("RowTitle = %q, want %q", got, c.want)
			}
		})
	}
}
