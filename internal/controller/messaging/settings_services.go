package messaging

import (
	"context"

	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/subcategory"
)

// settingsServices implements settings.Services for *controller. El resto de los
// métodos que la interfaz pide ya existen: los repos y el outbound los trae el
// runner de flow (controller.go) y el loop del agente (agent_services.go).
//
// La aserción de satisfacción vive en chat_bridge.go (settingsBridge), no acá:
// *controller ya no puede implementar settings.Services directamente — SendText/
// SendPrompt/StartFlow/HandleGroqError tienen la firma nueva de messenger.Chat,
// y settings todavía pide (bot, chatID) hasta que migre en la Task 7.

func (c *controller) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	return c.subcategories.DistinctCategoriesForUser(userID)
}

func (c *controller) FindOwnedSubcategories(userID uint64) ([]subcategory.Subcategory, error) {
	return c.subcategories.FindOwnedByUser(userID)
}

func (c *controller) TopDescriptionsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error) {
	return c.movements.TopDescriptionsBySubcategory(userID, subcategoryID, limit)
}

// RemindersFindByUserID ya está implementado en nudges_services.go (puente compartido).

func (c *controller) ResolveAccountManage(ctx context.Context, text string, accounts []orchestrator.AccountOption) (orchestrator.AccountManageResult, error) {
	return c.orchestrator.ResolveAccountManage(ctx, text, accounts)
}

func (c *controller) ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error) {
	return c.orchestrator.ClassifyOnboarding(ctx, text)
}

func (c *controller) ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error) {
	return c.orchestrator.ClassifyCategoryCreate(ctx, text, taxonomy)
}
