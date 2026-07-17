package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
)

// finishCategoryMatchOffer handles the "ya existe algo parecido" gate result:
// reuse the existing entry, fall through to the classic wizard to create a
// distinct one, or cancel.
func (c *controller) finishCategoryMatchOffer(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if flag(data, keyCancelled) {
		c.resolveMetric(ctx, data.UserID(), outcomeCategoryCancelled)
		c.sendText(ctx, b, chatID, msgCreateCancelled)
		return
	}
	switch stringOrEmpty(data[keyMatchChoice]) {
	case optionUseExisting:
		c.resolveMetric(ctx, data.UserID(), outcomeCategoryMatchUsed)
		c.sendText(ctx, b, chatID, msgCategoryMatchUse)
	case optionCreateNew:
		// stays pending in intent_events; the wizard's own terminal resolves it
		c.startSubcategoryWizard(ctx, b, chatID, data.UserID())
	default:
		c.sendText(ctx, b, chatID, msgGenericFlowError)
	}
}

// finishCategoryProposalConfirm handles the proposal confirmation: create it
// as-is, drop into the classic wizard seeded with the proposal to edit, or
// cancel.
func (c *controller) finishCategoryProposalConfirm(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if flag(data, keyCancelled) {
		c.resolveMetric(ctx, data.UserID(), outcomeCategoryCancelled)
		c.sendText(ctx, b, chatID, msgCreateCancelled)
		return
	}
	if flag(data, keyEditProposal) {
		seed := conversation.Data{
			keyCategory:               stringOrEmpty(data[keyCategory]),
			keyCategoryIsNew:          stringOrEmpty(data[keyCategoryIsNew]),
			keyCategoryIcon:           stringOrEmpty(data[keyCategoryIcon]),
			keySubcategory:            stringOrEmpty(data[keySubcategory]),
			keySubcategoryDescription: stringOrEmpty(data[keySubcategoryDescription]),
		}
		prompt, err := c.engine.StartWithData(data.UserID(), subcategorySetupFlowName, seed)
		if err != nil {
			c.sendText(ctx, b, chatID, msgGenericFlowError)
			return
		}
		c.sendPrompt(ctx, b, chatID, prompt)
		return
	}
	if err := c.insertNewSubcategory(data); err != nil {
		c.sendText(ctx, b, chatID, msgGenericFlowError)
		return
	}
	c.resolveMetric(ctx, data.UserID(), outcomeCategoryCreated)
	c.sendText(ctx, b, chatID, msgSubcategorySetupFinished)
}
