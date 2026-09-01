package movement

import (
	"strings"
	"testing"
)

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
