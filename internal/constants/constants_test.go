package constants

import "testing"

func TestStoredValuesAreFrozen(t *testing.T) {
	cases := map[string]string{
		"ARS": ARS, "USD": USD,
		"Expense": Expense, "Income": Income, "Transfer": Transfer,
		"PendingReview": PendingReview,
	}
	want := map[string]string{
		"ARS": "ARS", "USD": "USD",
		"Expense": "expense", "Income": "income", "Transfer": "transfer",
		"PendingReview": "PENDING_REVIEW",
	}
	for name, got := range cases {
		if got != want[name] {
			t.Errorf("%s = %q, want %q — this value is persisted; changing it corrupts existing rows", name, got, want[name])
		}
	}
}
