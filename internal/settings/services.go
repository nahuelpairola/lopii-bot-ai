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

type accountLister interface {
	FindByUserID(userID uint64) ([]account.Account, error)
}

type subcategoryReader interface {
	FindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	FindByCategoryAndSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
	DistinctCategoriesForUser(userID uint64) ([]string, error)
	FindOwnedByUser(userID uint64) ([]subcategory.Subcategory, error)
}

type descriptionLister interface {
	TopDescriptionsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error)
}

type reminderFinder interface {
	FindByUserID(userID uint64) (*reminder.Reminder, error)
}

type classifier interface {
	ResolveAccountManage(ctx context.Context, text string, accounts []orchestrator.AccountOption) (orchestrator.AccountManageResult, error)
	ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error)
	ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error)
}

type Wizards struct {
	accounts        accountLister
	subcategories   subcategoryReader
	movements       descriptionLister
	reminders       reminderFinder
	model           classifier
	engine          *conversation.Engine
	resolveMetric   func(ctx context.Context, userID uint64, outcome string, movementIDs ...uint)
	handleGroqError func(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error)
}

func New(
	accounts accountLister,
	subcategories subcategoryReader,
	movements descriptionLister,
	reminders reminderFinder,
	model classifier,
	engine *conversation.Engine,
	resolveMetric func(ctx context.Context, userID uint64, outcome string, movementIDs ...uint),
	handleGroqError func(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error),
) *Wizards {
	return &Wizards{
		accounts:        accounts,
		subcategories:   subcategories,
		movements:       movements,
		reminders:       reminders,
		model:           model,
		engine:          engine,
		resolveMetric:   resolveMetric,
		handleGroqError: handleGroqError,
	}
}
