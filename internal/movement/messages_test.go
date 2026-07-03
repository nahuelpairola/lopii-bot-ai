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

func TestTypeFromString(t *testing.T) {
	cases := map[string]movementType{"income": Income, "transfer": Transfer, "expense": Expense, "garbage": Expense}
	for input, want := range cases {
		if got := TypeFromString(input); got != want {
			t.Errorf("TypeFromString(%q) = %v, want %v", input, got, want)
		}
	}
}
