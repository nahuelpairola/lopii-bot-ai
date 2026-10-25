//go:build conv_test

package messaging

import (
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
)

func buttonWithLabel(t *testing.T, p conversation.Prompt, label string) string {
	t.Helper()
	for _, b := range p.Buttons {
		if strings.Contains(b.Label, label) {
			return b.Data
		}
	}
	t.Fatalf("no button %q in %q: %+v", label, p.Text, p.Buttons)
	return ""
}

func startUncategorizedOverdraft(h *convHarness) {
	h.ScriptCategory()
	h.ScriptToolCalls(recordMovementsCall(`{"movements":[
		{"type":"expense","amount":"2000000","currency":"ARS","description":"internet","date":"2026-10-23"}]}`))
	h.SendText("pagué internet 2000000")
}

func TestWizardCard_OneExpenseIsOneMessageThatTransformsIntoTheReceipt(t *testing.T) {
	h := newConversationHarness(t)
	startUncategorizedOverdraft(h)
	if len(h.chat.Sent) != 1 {
		t.Fatalf("sent = %+v, want the category picker as the only message", h.Messages())
	}

	h.TapButton(buttonWithLabel(t, h.lastShown(), "Vivienda"))
	h.TapButton(buttonWithLabel(t, h.lastShown(), "Internet"))
	h.TapButton(buttonWithLabel(t, h.lastShown(), "Registrar igual"))

	if len(h.chat.Sent) != 1 {
		t.Fatalf("sent = %+v, want every tapped step to rewrite the card instead of sending", h.Messages())
	}
	if len(h.chat.Edited) != 3 {
		t.Fatalf("edited %d times, want subcategory picker, overdraft warning and receipt: %+v", len(h.chat.Edited), h.chat.Edited)
	}
	if got := h.chat.Edited[2].Text; !strings.Contains(got, "Movimiento registrado") {
		t.Fatalf("last edit = %q, want the card to end as the receipt", got)
	}
	if len(h.Movements()) != 1 {
		t.Fatalf("movements = %d, want the expense registered once", len(h.Movements()))
	}
}

func TestWizardCard_ATypedAnswerGetsAFreshCard(t *testing.T) {
	h := newConversationHarness(t)
	startUncategorizedOverdraft(h)
	h.TapButton(buttonWithLabel(t, h.lastShown(), "Vivienda"))
	h.TapButton(buttonWithLabel(t, h.lastShown(), "Otra"))
	sentBefore := len(h.chat.Sent)

	h.Answer("Fibra")

	if len(h.chat.Sent) != sentBefore+1 {
		t.Fatalf("sent = %+v, want the step after a typed answer as a new message", h.Messages())
	}
}
