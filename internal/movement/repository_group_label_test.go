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

func TestGroupLabelExpr_CounterpartIsTheOtherLiveLegOfTheSameTransfer(t *testing.T) {
	got := groupLabelExpr(GroupByDirectionCounterpart)
	for _, want := range []string{
		"o.transaction_id = movements.transaction_id",
		"o.id <> movements.id",
		"o.deleted_at IS NULL",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("la contraparte tiene que ser la otra pata viva de la misma transferencia; falta %q en: %s", want, got)
		}
	}
}
