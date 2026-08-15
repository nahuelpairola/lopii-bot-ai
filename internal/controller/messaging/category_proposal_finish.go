package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

// finishCategoryMatchOffer handles the "ya existe algo parecido" gate result:
// reuse the existing entry, fall through to the classic wizard to create a
// distinct one, or cancel.
func (c *controller) finishCategoryMatchOffer(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		c.resolveMetric(ctx, data.UserID(), outcomeCategoryCancelled)
		c.sendText(ctx, b, chatID, msgCreateCancelled)
		return
	}
	switch conversation.StringOrEmpty(data[conversation.KeyMatchChoice]) {
	case flow.OptionUseExisting:
		c.resolveMetric(ctx, data.UserID(), outcomeCategoryMatchUsed)
		c.sendText(ctx, b, chatID, msgCategoryMatchUse)
	case flow.OptionCreateNew:
		// stays pending in intent_events; the wizard's own terminal resolves it
		c.startSubcategoryWizard(ctx, b, chatID, data.UserID())
	default:
		c.sendText(ctx, b, chatID, msgSomethingBroke)
	}
}

// finishCategoryProposalConfirm handles the proposal confirmation: create it
// as-is, drop into the classic wizard seeded with the proposal to edit, or
// cancel.
func (c *controller) finishCategoryProposalConfirm(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		c.resolveMetric(ctx, data.UserID(), outcomeCategoryCancelled)
		c.sendText(ctx, b, chatID, msgCreateCancelled)
		return
	}
	if conversation.Flag(data, conversation.KeyEditProposal) {
		seed := conversation.Data{
			conversation.KeyCategory:               conversation.StringOrEmpty(data[conversation.KeyCategory]),
			conversation.KeyCategoryIsNew:          conversation.StringOrEmpty(data[conversation.KeyCategoryIsNew]),
			conversation.KeyCategoryIcon:           conversation.StringOrEmpty(data[conversation.KeyCategoryIcon]),
			conversation.KeySubcategory:            conversation.StringOrEmpty(data[conversation.KeySubcategory]),
			conversation.KeySubcategoryDescription: conversation.StringOrEmpty(data[conversation.KeySubcategoryDescription]),
		}
		prompt, err := c.engine.StartWithData(data.UserID(), flow.SubcategorySetupFlowName, seed)
		if err != nil {
			c.sendText(ctx, b, chatID, msgSomethingBroke)
			return
		}
		c.sendPrompt(ctx, b, chatID, prompt)
		return
	}
	if err := c.insertNewSubcategory(data); err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotSave("tu categoría"))
		return
	}
	c.resolveMetric(ctx, data.UserID(), outcomeCategoryCreated)
	c.sendText(ctx, b, chatID, msgSubcategorySetupFinished)
}
