package movement

import "testing"

func TestIconForType(t *testing.T) {
	cases := []struct {
		typ  movementType
		want string
	}{
		{Expense, "🔴"},
		{Income, "🟢"},
		{Transfer, "🏦"},
	}
	for _, c := range cases {
		if got := IconForType(c.typ); got != c.want {
			t.Errorf("IconForType(%v) = %q, want %q", c.typ, got, c.want)
		}
	}
}
