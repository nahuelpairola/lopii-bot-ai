package messaging

import (
	"context"
	"encoding/json"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/chathistory"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/query"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
)

// Los métodos de este archivo implementan query.services: lo que el loop de
// QUERY necesita del mundo, con el *controller* como implementación. Puentes
// de una línea, como en agent_services.go — el loop no importa los repos del
// borde. handleQuery, abajo, es el delegado del controller hacia query.Run.

func (c *controller) QueryAccountsByUserID(userID uint64) ([]account.Account, error) {
	return c.accounts.FindByUserID(userID)
}
func (c *controller) QueryCategoriesByUser(userID uint64) ([]subcategory.Subcategory, error) {
	return c.subcategories.FindAllForUser(userID)
}
func (c *controller) QueryIconForCategory(userID uint64, category string) string {
	return c.subcategories.IconForCategory(userID, category)
}
func (c *controller) QueryListMovements(q movement.MovementQuery, limit int) ([]movement.Movement, error) {
	return c.movements.ListForUser(q, limit)
}
func (c *controller) QuerySumMovements(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
	return c.movements.SumForUser(q, groupBy)
}
func (c *controller) QueryBalanceForAccount(accountID uint64) (decimal.Decimal, error) {
	return c.movements.SumAmountForAccount(accountID)
}
func (c *controller) QueryReminderByUser(userID uint64) (*reminder.Reminder, error) {
	return c.reminders.FindByUserID(userID)
}
func (c *controller) QueryChatRecent(userID uint64) ([]chathistory.Turn, error) {
	return c.chatHistory.Recent(userID)
}
func (c *controller) QueryChatAppend(userID uint64, question, answer string) error {
	return c.chatHistory.Append(userID, question, answer)
}
func (c *controller) QuerySendText(ctx context.Context, chat messenger.Chat, text string) {
	_ = messenger.SendText(ctx, chat, text)
}
func (c *controller) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	return c.orchestrator.AnswerQuery(ctx, systemPrompt, userText, history, tools, execute)
}

// handleQuery entrega la consulta al loop de QUERY. Conserva el contrato de
// siempre (answered, err) — finishAnswerQuery y nudge dependen de ambos.
// *controller implementa la interfaz no exportada query.services
// directamente desde la Task 7 — QuerySendText ya sólo pide messenger.Chat,
// así que no hace falta un puente.
func (c *controller) handleQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) (bool, error) {
	return query.Run(ctx, c, chat, userID, text)
}
