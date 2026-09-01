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

type Services interface {
	FindUserAccounts(userID uint64) ([]account.Account, error)
	SubcategoriesFindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	FindSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
	DistinctCategoriesForUser(userID uint64) ([]string, error)
	FindOwnedSubcategories(userID uint64) ([]subcategory.Subcategory, error)
	TopDescriptionsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error)
	RemindersFindByUserID(userID uint64) (*reminder.Reminder, error)

	ResolveAccountManage(ctx context.Context, text string, accounts []orchestrator.AccountOption) (orchestrator.AccountManageResult, error)
	ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error)
	ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error)

	SendText(ctx context.Context, chat messenger.Chat, text string)
	SendPrompt(ctx context.Context, chat messenger.Chat, prompt conversation.Prompt)
	StartFlow(ctx context.Context, chat messenger.Chat, userID uint64, flowName string, seed conversation.Data, errCtx string) error
	EngineStartWithData(userID uint64, flowName string, seed conversation.Data) (conversation.Prompt, error)
	ResolveMetric(ctx context.Context, userID uint64, outcome string, movementIDs ...uint)
	HandleGroqError(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error)
}
