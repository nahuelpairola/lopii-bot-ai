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

const enrichBase = "¿Anotamos lo de hoy?"

func TestEnrich_FewerThanTwoItemsReturnsBaseUnchanged(t *testing.T) {
	for _, items := range [][]string{nil, {}, {"café"}} {
		if got := Enrich(enrichBase, items); got != enrichBase {
			t.Errorf("Enrich(%v) = %q, want base unchanged", items, got)
		}
	}
}

func TestEnrich_TwoItemsJoinWithO(t *testing.T) {
	want := enrichBase + "\n\nPor acá suele haber café o panadería."
	if got := Enrich(enrichBase, []string{"café", "panadería"}); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEnrich_ThreeItemsJoinWithCommaAndO(t *testing.T) {
	want := enrichBase + "\n\nPor acá suele haber café, panadería o verdulería."
	if got := Enrich(enrichBase, []string{"café", "panadería", "verdulería"}); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEnrich_RenderedMessageEscapesUserDescriptions(t *testing.T) {
	got := Enrich(enrichBase, []string{"Ahorro & Cía", "<b>kiosco</b>"})
	want := enrichBase + "\n\nPor acá suele haber Ahorro &amp; Cía o &lt;b&gt;kiosco&lt;/b&gt;."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
