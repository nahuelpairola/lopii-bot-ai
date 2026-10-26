package messaging

import (
	"lopiibot.com/internal/query"
)

func (c *controller) queries() *query.Answerer {
	return query.New(c.accounts, c.movements, c.subcategories, c.reminders, c.chatHistory, c.orchestrator)
}
