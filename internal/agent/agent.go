package agent

import (
	"context"
	"encoding/json"
	"time"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/subcategory"
)

type agentServices interface {
	FindUserAccounts(userID uint64) ([]account.Account, error)
	AccountsHasDefaultForCurrency(userID uint64, cur currency.Currency) bool
	FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error)
	MovementsFindSimilarForUser(userID uint64, message string, since time.Time, until *time.Time) ([]movement.Movement, error)
	SubcategoriesFindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	FindSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
	ChatHistoryRecent(userID uint64) ([]chathistory.Turn, error)
	ChatHistoryAppend(userID uint64, question, answer string) error

	ActionsInsert(a *pendingaction.PendingAction) error
	ActionsNextForUser(userID uint64) (*pendingaction.PendingAction, error)
	ActionsUpdate(a *pendingaction.PendingAction) error
	ActionsDelete(id uint64) error
	ActionsEnabled() bool

	MetricsLog(userID uint64, traceID, rawMessage, intent string, needsConfirmation bool, outcome string) error
	ResolveMetric(ctx context.Context, userID uint64, outcome string, movementIDs ...uint)
	MetricsSetIntentIfQueued(userID uint64, intent string) error

	Run(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error)
	ClassifyCategories(ctx context.Context, message string, rows []orchestrator.ClassifyRow, taxonomy []orchestrator.TaxonomyEntry) []orchestrator.Pair
	ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error)

	SendText(ctx context.Context, chat messenger.Chat, text string)
	SendPrompt(ctx context.Context, chat messenger.Chat, prompt conversation.Prompt)
	StartFlow(ctx context.Context, chat messenger.Chat, userID uint64, flowName string, seed conversation.Data, errCtx string) error
	EngineStartWithData(userID uint64, flowName string, seed conversation.Data) (conversation.Prompt, error)
	IsReplaying(ctx context.Context) bool

	MaybeNearDuplicate(userID uint64, inserted []movement.Movement) []conversation.Button
	ResolveAndInsertMovements(data conversation.Data) ([]movement.Movement, error)

	HandleGroqError(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error)
	EnqueueUpdatePickIfRateLimited(ctx context.Context, chat messenger.Chat, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool
	FinishAnswerQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) error
	FinishManageSettings(ctx context.Context, chat messenger.Chat, userID uint64, text, area string) error
}

func StartLoop(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, text string) error {
	return startAgentLoop(ctx, svc, chat, userID, text)
}

func FinishAskUser(ctx context.Context, svc agentServices, chat messenger.Chat, data conversation.Data) {
	finishAskUserFlow(ctx, svc, chat, data)
}

func FinishMovementUpdatePick(ctx context.Context, svc agentServices, chat messenger.Chat, data conversation.Data) {
	finishMovementUpdatePickFlow(ctx, svc, chat, data)
}

func ProceedToUpdateConfirm(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, ask ChangeAsk) error {
	return proceedToUpdateConfirm(ctx, svc, chat, userID, message, transactionID, oldIDs, beforeRows, ask)
}

func DrainNextAction(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64) error {
	return drainNextAgentAction(ctx, svc, chat, userID)
}

const (
	SettingsAreaAccount        = "cuenta"
	SettingsAreaCategory       = "categoria"
	SettingsAreaCategoryManage = "categoria_administrar"
	SettingsAreaReminder       = "recordatorio"
)
