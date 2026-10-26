package messaging

import "lopiibot.com/internal/nudges"

func (c *controller) tips() *nudges.Tips {
	return nudges.New(c.engine, c.reminders, c.users, c.accounts, c.subcategories, c.movements, c.nudges, c.queries().Run)
}
