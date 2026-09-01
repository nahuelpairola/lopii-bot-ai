package messenger

import (
	"context"

	"lopiibot.com/internal/conversation"
)

type FakeChat struct {
	Sent  []conversation.Prompt
	Typed int
	Err   error
}

func (f *FakeChat) Send(_ context.Context, p conversation.Prompt) error {
	if f.Err != nil {
		return f.Err
	}
	f.Sent = append(f.Sent, p)
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
