package messaging

import (
	"context"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/query"
)

func (c *controller) handleQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) (bool, error) {
	return c.queries().Run(ctx, chat, userID, text)
}

func (c *controller) queries() *query.Answerer {
	return query.New(c.accounts, c.movements, c.subcategories, c.reminders, c.chatHistory, c.orchestrator)
}
