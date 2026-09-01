package reminder

import (
	"strings"
	"testing"
)

func TestPickMessageNonEmpty(t *testing.T) {
	for i := 0; i < 50; i++ {
		if strings.TrimSpace(PickMessage()) == "" {
			t.Fatal("PickMessage returned empty string")
		}
	}
}

func TestReminderMessagesPoolHasVariety(t *testing.T) {
	if len(reminderMessages) < 10 {
		t.Errorf("pool has %d messages, want >= 10 for variety", len(reminderMessages))
	}
	seen := map[string]bool{}
	for _, m := range reminderMessages {
		if seen[m] {
			t.Errorf("duplicate message in pool: %q", m)
		}
		seen[m] = true
	}
}

func TestReminderMessagesAreHTMLSafe(t *testing.T) {
	for _, m := range reminderMessages {
		if strings.ContainsAny(m, "<>&") {
			t.Errorf("mensaje con carácter especial de HTML: %q", m)
		}
	}
}
