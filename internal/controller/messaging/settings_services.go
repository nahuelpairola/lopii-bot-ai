package messaging

import (
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/settings"
)

func (c *controller) wizards() *settings.Wizards {
	return settings.New(c.accounts, c.subcategories, c.movements, c.reminders, c.orchestrator, c.engine, c.resolveMetric, c.HandleGroqError)
}

func (c *controller) finisher() *flow.Finisher {
	w := c.wizards()
	return flow.New(c.accounts, c.movements, c.subcategories, c.reminders, c.nudges, c.engine, c.resolveMetric, w.StartAccountCreate, w.SuggestMergeTarget)
}
