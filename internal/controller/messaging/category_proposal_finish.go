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
	if stringOrEmpty(data["cancelled"]) == "true" {
		c.resolveMetric(data.UserID(), outcomeCategoryCancelled)
		c.sendText(ctx, b, chatID, msgCreateCancelled)
		return
	}
	switch stringOrEmpty(data["match_choice"]) {
	case optionUseExisting:
		c.resolveMetric(data.UserID(), outcomeCategoryMatchUsed)
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
	if stringOrEmpty(data["cancelled"]) == "true" {
		c.resolveMetric(data.UserID(), outcomeCategoryCancelled)
		c.sendText(ctx, b, chatID, msgCreateCancelled)
		return
	}
	if stringOrEmpty(data["edit_proposal"]) == "true" {
		seed := conversation.Data{
			"category":                stringOrEmpty(data["category"]),
			"category_is_new":         stringOrEmpty(data["category_is_new"]),
			"category_icon":           stringOrEmpty(data["category_icon"]),
			"subcategory":             stringOrEmpty(data["subcategory"]),
			"subcategory_description": stringOrEmpty(data["subcategory_description"]),
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
	c.resolveMetric(data.UserID(), outcomeCategoryCreated)
	c.sendText(ctx, b, chatID, msgSubcategorySetupFinished)
}
