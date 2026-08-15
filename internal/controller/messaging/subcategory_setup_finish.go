package messaging

import (
	"context"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

// finishSubcategorySetupFlow is the Telegram-facing wrapper around
// insertNewSubcategory — same split as finishAccountCreateFlow: no DB
// write happens if the user cancelled (this flow's own Cancelar button,
// checked via data["cancelled"] — independent from the resume-gate's
// separate data["_resume_cancelled"] marker, per the engine-level
// short-circuit in handleFlowFinished).
func (c *controller) finishSubcategorySetupFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if conversation.Flag(data, conversation.KeyCancelled) {
		c.resolveMetric(ctx, data.UserID(), outcomeCategoryCancelled)
		c.sendText(ctx, b, chatID, msgCreateCancelled)
		return
	}

	if err := c.insertNewSubcategory(data); err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotSave("tu categoría"))
		return
	}

	c.resolveMetric(ctx, data.UserID(), outcomeCategoryCreated)
	c.sendText(ctx, b, chatID, msgSubcategorySetupFinished)
}

// insertNewSubcategory does the real work (kept separate from the
// Telegram-facing wrapper so it's testable without *bot.Bot, same
// pattern as insertAccountOpeningMovement/insertInitialBalanceMovements).
// When category_is_new is "true", the new category's icon comes from the
// step the user just answered (category_icon); otherwise it's copied
// from the existing category's icon (found via the same Cache lookup
// used for the duplicate check) — never re-asked. Description comes
// straight from stepSubcategoryDescription's answer — this is the field
// orchestrator.TaxonomyEntry.Description feeds to Call 2 CREATE as a
// classification hint, so a user-created subcategory is only as useful
// as this description is specific.
func (c *controller) insertNewSubcategory(data conversation.Data) error {
	userID := data.UserID()
	category := conversation.StringOrEmpty(data[conversation.KeyCategory])
	sub := conversation.StringOrEmpty(data[conversation.KeySubcategory])
	description := conversation.StringOrEmpty(data[conversation.KeySubcategoryDescription])

	icon := conversation.StringOrEmpty(data[conversation.KeyCategoryIcon])
	if icon == "" {
		icon = c.subcategories.IconForCategory(userID, category)
	}

	s := &subcategory.Subcategory{
		UserID:      &userID,
		Category:    category,
		Subcategory: sub,
		Description: description,
		IsGlobal:    false,
		Icon:        icon,
	}
	if err := c.subcategories.Insert(s); err != nil {
		return err
	}
	return c.subcategories.Reload()
}
