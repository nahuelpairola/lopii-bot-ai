package messaging

import "lopiibot.com/internal/settings"

func (c *controller) wizards() *settings.Wizards {
	return settings.New(c.accounts, c.subcategories, c.movements, c.reminders, c.orchestrator, c.engine, c.resolveMetric, c.HandleGroqError)
}
