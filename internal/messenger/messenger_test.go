package messenger

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
)

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

func TestFakeChat_ErrShortCircuitsBeforeRecording(t *testing.T) {
	f := &FakeChat{Err: context.DeadlineExceeded}
	if err := f.Send(context.Background(), conversation.Prompt{Text: "hola"}); err == nil {
		t.Fatal("Send: quería el error simulado")
	}
	if len(f.Sent) != 0 {
		t.Errorf("Sent = %+v, quería vacío tras error", f.Sent)
	}
	if err := f.Typing(context.Background()); err == nil {
		t.Fatal("Typing: quería el error simulado")
	}
	if f.Typed != 0 {
		t.Errorf("Typed = %d, quería 0 tras error", f.Typed)
	}
}

type sendOnlyChat struct{ sent []conversation.Prompt }

func (c *sendOnlyChat) Send(_ context.Context, p conversation.Prompt) error {
	c.sent = append(c.sent, p)
	return nil
}

func (c *sendOnlyChat) Typing(context.Context) error { return nil }

func TestEditSource_UsesTheChatsEditorWhenItHasOne(t *testing.T) {
	fc := &FakeChat{}
	p := conversation.Prompt{Text: "📅 Agendado"}
	if err := EditSource(context.Background(), fc, p); err != nil {
		t.Fatal(err)
	}
	if len(fc.Edited) != 1 || fc.Edited[0].Text != p.Text || len(fc.Sent) != 0 {
		t.Fatalf("edited=%v sent=%v, want one edit and no new message", fc.Edited, fc.Sent)
	}
}

func TestEditSource_FallsBackToSendOnAChannelThatCannotEdit(t *testing.T) {
	c := &sendOnlyChat{}
	if err := EditSource(context.Background(), c, conversation.Prompt{Text: "x"}); err != nil {
		t.Fatal(err)
	}
	if len(c.sent) != 1 {
		t.Fatalf("sent %d, want the outcome as a new message", len(c.sent))
	}
}

func TestClearSourceButtons_IsANoOpOnAChannelThatCannotEdit(t *testing.T) {
	c := &sendOnlyChat{}
	if err := ClearSourceButtons(context.Background(), c); err != nil || len(c.sent) != 0 {
		t.Fatalf("err=%v sent=%v, want nothing", err, c.sent)
	}
	fc := &FakeChat{}
	_ = ClearSourceButtons(context.Background(), fc)
	if fc.Cleared != 1 {
		t.Fatalf("cleared = %d, want 1", fc.Cleared)
	}
}
