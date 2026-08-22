package messenger

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
)

// SendText es un helper de paquete y no un método de la interfaz: un Prompt
// sin botones YA es un texto. Este test fija esa equivalencia.
func TestSendText_IsAPromptWithNoButtons(t *testing.T) {
	f := &FakeChat{}
	if err := SendText(context.Background(), f, "hola"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if len(f.Sent) != 1 {
		t.Fatalf("mandó %d prompts, quería 1", len(f.Sent))
	}
	if f.Sent[0].Text != "hola" {
		t.Errorf("Text = %q, want %q", f.Sent[0].Text, "hola")
	}
	if len(f.Sent[0].Buttons) != 0 {
		t.Errorf("un texto no lleva botones, llevó %d", len(f.Sent[0].Buttons))
	}
}

func TestFakeChat_RecordsPromptsAndTyping(t *testing.T) {
	f := &FakeChat{}
	p := conversation.Prompt{Text: "elegí", Buttons: []conversation.Button{{Label: "sí", Data: "yes"}}}
	if err := f.Send(context.Background(), p); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := f.Typing(context.Background()); err != nil {
		t.Fatalf("Typing: %v", err)
	}
	if len(f.Sent) != 1 || f.Sent[0].Buttons[0].Data != "yes" {
		t.Errorf("no guardó el prompt con botones: %+v", f.Sent)
	}
	if f.Typed != 1 {
		t.Errorf("Typed = %d, want 1", f.Typed)
	}
}
