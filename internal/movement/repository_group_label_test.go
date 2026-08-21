package movement

import (
	"strings"
	"testing"
)

// El CASE es lo que evita el doble conteo. Un transfer son DOS filas —la que
// sale y la que entra— y SumForUser suma ABS, así que sin partir por dirección
// "cuánto transferí este mes" devuelve exactamente el doble. Medido en prod el
// 2026-08-21: 14 patas, $2.897.191,18 = 2 × $1.448.595,59.
func TestGroupLabelExpr_Direction(t *testing.T) {
	got := groupLabelExpr(GroupByDirection)
	if got == "" {
		t.Fatal("GroupByDirection no está mapeada")
	}
	for _, want := range []string{"movements.amount < 0", "'out'", "'in'"} {
		if !strings.Contains(got, want) {
			t.Errorf("la expresión no contiene %q: %s", want, got)
		}
	}
}
