package messenger

import (
	"context"

	"lopiibot.com/internal/conversation"
)

type FakeChat struct {
	Sent    []conversation.Prompt
	Edited  []conversation.Prompt
	Shown   []conversation.Prompt
	Cleared int
	Typed   int
	Err     error
}

func (f *FakeChat) EditSource(_ context.Context, p conversation.Prompt) error {
	if f.Err != nil {
		return f.Err
	}
	f.Edited = append(f.Edited, p)
	f.Shown = append(f.Shown, p)
	return nil
}

func (f *FakeChat) ClearSourceButtons(context.Context) error {
	if f.Err != nil {
		return f.Err
	}
	f.Cleared++
	return nil
}

func (f *FakeChat) Send(_ context.Context, p conversation.Prompt) error {
	if f.Err != nil {
		return f.Err
	}
	f.Sent = append(f.Sent, p)
	f.Shown = append(f.Shown, p)
	return nil
}

func (f *FakeChat) Typing(context.Context) error {
	if f.Err != nil {
		return f.Err
	}
	f.Typed++
	return nil
}

func (f *FakeChat) LastText() string {
	if len(f.Sent) == 0 {
		return ""
	}
	return f.Sent[len(f.Sent)-1].Text
}
