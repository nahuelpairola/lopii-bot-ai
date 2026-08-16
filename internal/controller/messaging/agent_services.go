package messaging

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/subcategory"
)

// Los métodos de este archivo implementan agent.agentServices: lo que el loop
// unificado necesita del mundo, con el *controller* como implementación. Los que
// ya existían por el runner de flow (FindUserAccounts, FindSubcategory,
// ResolveMetric, SendText, StartFlow, FindRecentlyCreatedForUser) viven en
// controller.go; acá están los que no había. Puentes de una línea, como en el
// runner — el loop no importa los repos del borde.

func (c *controller) AccountsHasDefaultForCurrency(userID uint64, cur currency.Currency) bool {
	return c.accounts.HasDefaultForCurrency(userID, cur)
}

func (c *controller) MovementsFindSimilarForUser(userID uint64, message string, since time.Time, until *time.Time) ([]movement.Movement, error) {
	return c.movements.FindSimilarForUser(userID, message, since, until)
}

func (c *controller) SubcategoriesFindAllForUser(userID uint64) ([]subcategory.Subcategory, error) {
	return c.subcategories.FindAllForUser(userID)
}

func (c *controller) ChatHistoryRecent(userID uint64) ([]chathistory.Turn, error) {
	return c.chatHistory.Recent(userID)
}

func (c *controller) ChatHistoryAppend(userID uint64, question, answer string) error {
	return c.chatHistory.Append(userID, question, answer)
}

func (c *controller) ActionsInsert(a *pendingaction.PendingAction) error {
	if c.actions == nil {
		return nil
	}
	return c.actions.Insert(a)
}

func (c *controller) ActionsNextForUser(userID uint64) (*pendingaction.PendingAction, error) {
	if c.actions == nil {
		return nil, pendingaction.ErrNoPendingAction
	}
	return c.actions.NextForUser(userID)
}

func (c *controller) ActionsDelete(id uint64) error {
	if c.actions == nil {
		return nil
	}
	return c.actions.Delete(id)
}

func (c *controller) ActionsEnabled() bool {
	return c.actions != nil
}

func (c *controller) MetricsLog(userID uint64, traceID, rawMessage, intent string, needsConfirmation bool, outcome string) error {
	if c.metrics == nil {
		return nil
	}
	return c.metrics.Log(userID, traceID, rawMessage, intent, needsConfirmation, outcome)
}

func (c *controller) MetricsSetIntentIfQueued(userID uint64, intent string) error {
	if c.metrics == nil {
		return nil
	}
	return c.metrics.SetIntentIfQueued(userID, intent)
}

func (c *controller) Run(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	return c.orchestrator.Run(ctx, systemPrompt, userText, history, tools, execute)
}

func (c *controller) ClassifyCategories(ctx context.Context, message string, rows []orchestrator.ClassifyRow, taxonomy []orchestrator.TaxonomyEntry) []orchestrator.Pair {
	return c.orchestrator.ClassifyCategories(ctx, message, rows, taxonomy)
}

func (c *controller) ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error) {
	return c.orchestrator.ResolveUpdate(ctx, text, candidate, accounts)
}

func (c *controller) SendPrompt(ctx context.Context, b *bot.Bot, chatID int64, prompt conversation.Prompt) {
	c.sendPrompt(ctx, b, chatID, prompt)
}

func (c *controller) EngineStartWithData(userID uint64, flowName string, seed conversation.Data) (conversation.Prompt, error) {
	return c.engine.StartWithData(userID, flowName, seed)
}

func (c *controller) IsReplaying(ctx context.Context) bool {
	return isReplaying(ctx)
}

func (c *controller) MaybeNearDuplicate(userID uint64, inserted []movement.Movement) []conversation.Button {
	return flow.MaybeNearDuplicate(c, userID, inserted)
}

func (c *controller) ResolveAndInsertMovements(data conversation.Data) ([]movement.Movement, error) {
	return flow.ResolveAndInsertMovements(c, data)
}

func (c *controller) HandleGroqError(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string, err error) (bool, error) {
	return c.handleGroqError(ctx, b, chatID, userID, text, err)
}

func (c *controller) EnqueueUpdatePickIfRateLimited(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool {
	return c.enqueueUpdatePickIfRateLimited(ctx, b, chatID, userID, message, transactionID, oldIDs, beforeRows, err)
}

func (c *controller) FinishAnswerQuery(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	return c.finishAnswerQuery(ctx, b, chatID, userID, text)
}

func (c *controller) FinishManageSettings(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text, area string) error {
	return c.finishManageSettings(ctx, b, chatID, userID, text, area)
}
