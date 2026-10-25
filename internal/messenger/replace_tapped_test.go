package messenger

import (
	"context"
	"errors"
	"testing"

	"lopiibot.com/internal/conversation"
)

func TestReplaceTapped_FirstSendRewritesTheTappedMessageAndTheRestAreNew(t *testing.T) {
	fc := &FakeChat{}
	c := ReplaceTapped(fc)
	ctx := context.Background()

	if err := c.Send(ctx, conversation.Prompt{Text: "¿Y la subcategoría?"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(ctx, conversation.Prompt{Text: "💡 tip"}); err != nil {
		t.Fatal(err)
	}

	if len(fc.Edited) != 1 || fc.Edited[0].Text != "¿Y la subcategoría?" {
		t.Fatalf("edited = %v, want the first prompt as the one edit", fc.Edited)
	}
	if len(fc.Sent) != 1 || fc.Sent[0].Text != "💡 tip" {
		t.Fatalf("sent = %v, want only the second prompt as a new message", fc.Sent)
	}
}

func TestReplaceTapped_AnExplicitEditUsesUpTheTurnsOneEdit(t *testing.T) {
	fc := &FakeChat{}
	c := ReplaceTapped(fc)
	ctx := context.Background()

	if err := EditSource(ctx, c, conversation.Prompt{Text: "✅ Hecho"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearSourceButtons(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(ctx, conversation.Prompt{Text: "✅ Movimiento registrado"}); err != nil {
		t.Fatal(err)
	}

	if len(fc.Edited) != 1 || fc.Edited[0].Text != "✅ Hecho" {
		t.Fatalf("edited = %v, want only the explicit edit", fc.Edited)
	}
	if fc.Cleared != 1 {
		t.Fatalf("cleared = %d, want the clear to reach the tapped message", fc.Cleared)
	}
	if len(fc.Sent) != 1 || fc.Sent[0].Text != "✅ Movimiento registrado" {
		t.Fatalf("sent = %v, want the receipt as a new message after the explicit edit", fc.Sent)
	}
}

func TestReplaceTapped_TypingDoesNotUseUpTheEdit(t *testing.T) {
	fc := &FakeChat{}
	c := ReplaceTapped(fc)
	ctx := context.Background()

	if err := c.Typing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(ctx, conversation.Prompt{Text: "⚠️ Ojo"}); err != nil {
		t.Fatal(err)
	}

	if fc.Typed != 1 || len(fc.Edited) != 1 || len(fc.Sent) != 0 {
		t.Fatalf("typed=%d edited=%v sent=%v, want the prompt after typing to still be the edit", fc.Typed, fc.Edited, fc.Sent)
	}
}

type uneditableChat struct {
	sendOnlyChat
}

func (c *uneditableChat) EditSource(context.Context, conversation.Prompt) error {
	return errors.New("message can't be edited")
}

func (c *uneditableChat) ClearSourceButtons(context.Context) error { return nil }

func TestReplaceTapped_AFailedEditStillReachesTheUserAsANewMessage(t *testing.T) {
	uc := &uneditableChat{}
	if err := ReplaceTapped(uc).Send(context.Background(), conversation.Prompt{Text: "✅ Movimiento registrado"}); err != nil {
		t.Fatalf("Send = %v, want the fallback to succeed", err)
	}
	if len(uc.sent) != 1 || uc.sent[0].Text != "✅ Movimiento registrado" {
		t.Fatalf("sent = %v, want the step as a new message", uc.sent)
	}
}
