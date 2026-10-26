package messaging

import (
	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/settings"
)

func (c *controller) wizards() *settings.Wizards {
	return settings.New(c.accounts, c.subcategories, c.movements, c.reminders, c.orchestrator, c.engine, c.resolveMetric, c.handleGroqError)
}

func (c *controller) finisher() *flow.Finisher {
	w := c.wizards()
	return flow.New(c.accounts, c.movements, c.subcategories, c.reminders, c.nudges, c.engine, c.resolveMetric, w.StartAccountCreate, w.SuggestMergeTarget)
}

func (c *controller) loop() *agent.Loop {
	return agent.New(c.accounts, c.movements, c.subcategories, c.chatHistory, c.actions, c.metrics, c.orchestrator, c.engine, c.finisher(),
		c.resolveMetric, c.handleGroqError, c.enqueueUpdatePick, c.finishAnswerQuery, c.wizards().Dispatch)
}
