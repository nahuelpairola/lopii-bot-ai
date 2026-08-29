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

func TestGroupRowsByDay(t *testing.T) {
	t.Run("agrupa días consecutivos y respeta el orden", func(t *testing.T) {
		rows := []MovementRow{
			{Title: "Súper", Date: "12 ago"},
			{Title: "Nafta", Date: "12 ago"},
			{Title: "Sueldo", Date: "11 ago"},
		}

		got := GroupRowsByDay(rows)

		if len(got) != 2 {
			t.Fatalf("grupos = %d, want 2: %+v", len(got), got)
		}
		if got[0].Date != "12 ago" || len(got[0].Rows) != 2 {
			t.Errorf("primer grupo = %+v, want 12 ago con 2 filas", got[0])
		}
		if got[1].Date != "11 ago" || len(got[1].Rows) != 1 {
			t.Errorf("segundo grupo = %+v, want 11 ago con 1 fila", got[1])
		}
		if got[0].Rows[0].Title != "Súper" || got[0].Rows[1].Title != "Nafta" {
			t.Errorf("se perdió el orden del repositorio: %+v", got[0].Rows)
		}
	})

	t.Run("dos dias iguales no adyacentes no se fusionan", func(t *testing.T) {
		rows := []MovementRow{
			{Title: "de este año", Date: "12 ago"},
			{Title: "del medio", Date: "3 ene"},
			{Title: "del año pasado", Date: "12 ago"},
		}

		got := GroupRowsByDay(rows)

		if len(got) != 3 {
			t.Fatalf("grupos = %d, want 3 — un map los habría fusionado: %+v", len(got), got)
		}
	})

	t.Run("sin filas no hay grupos", func(t *testing.T) {
		if got := GroupRowsByDay(nil); len(got) != 0 {
			t.Errorf("GroupRowsByDay(nil) = %+v, want vacío", got)
		}
	})
}
