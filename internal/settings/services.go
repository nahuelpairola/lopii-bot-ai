package settings

import (
	"context"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
)

// Services is what the settings wizards need from the world, with the
// *controller as implementation. Bridge file: messaging/settings_services.go.
//
// Casi todos estos métodos ya existían para el runner de flow o para el loop
// del agente — este cluster no agregó puentes nuevos salvo los cuatro de LLM y
// los tres de lectura que sólo él usa.
type Services interface {
	// Repos
	FindUserAccounts(userID uint64) ([]account.Account, error)
	SubcategoriesFindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	FindSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
	DistinctCategoriesForUser(userID uint64) ([]string, error)
	FindOwnedSubcategories(userID uint64) ([]subcategory.Subcategory, error)
	TopDescriptionsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error)
	RemindersFindByUserID(userID uint64) (*reminder.Reminder, error)

	// LLM — este cluster es el único lado de la app que llama a estas tres.
	ResolveAccountManage(ctx context.Context, text string, accounts []orchestrator.AccountOption) (orchestrator.AccountManageResult, error)
	ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error)
	ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error)

	// Outbound
	SendText(ctx context.Context, chat messenger.Chat, text string)
	SendPrompt(ctx context.Context, chat messenger.Chat, prompt conversation.Prompt)
	StartFlow(ctx context.Context, chat messenger.Chat, userID uint64, flowName string, seed conversation.Data, errCtx string) error
	EngineStartWithData(userID uint64, flowName string, seed conversation.Data) (conversation.Prompt, error)
	ResolveMetric(ctx context.Context, userID uint64, outcome string, movementIDs ...uint)
	// HandleGroqError atiende el 429 antes que cualquier fallback: sin esto un
	// problema de cupo se disfraza de "no te entendí". Ver internal/pendingjob.
	HandleGroqError(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error)
}
