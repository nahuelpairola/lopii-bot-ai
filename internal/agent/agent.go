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

type accountFinder interface {
	FindByUserID(userID uint64) ([]account.Account, error)
	HasDefaultForCurrency(userID uint64, cur currency.Currency) bool
}

type movementFinder interface {
	FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error)
	FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error)
}

type subcategoryFinder interface {
	FindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	FindByCategoryAndSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
}

type chatHistory interface {
	Recent(userID uint64) ([]chathistory.Turn, error)
	Append(userID uint64, question, answer string) error
}

type actionQueue interface {
	Insert(action *pendingaction.PendingAction) error
	NextForUser(userID uint64) (*pendingaction.PendingAction, error)
	Update(action *pendingaction.PendingAction) error
	Delete(id uint64) error
}

type metricLog interface {
	Log(userID uint64, traceID, rawMessage, intent string, needsConfirmation bool, outcome string) error
	SetIntentIfQueued(userID uint64, intent string) error
}

type loopModel interface {
	Run(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error)
	ClassifyCategories(ctx context.Context, message string, rows []orchestrator.ClassifyRow, taxonomy []orchestrator.TaxonomyEntry) []orchestrator.Pair
	ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error)
}

type movementWriter interface {
	ResolveAndInsertMovements(data conversation.Data) ([]movement.Movement, error)
	MaybeNearDuplicate(userID uint64, inserted []movement.Movement) []conversation.Button
}

type Loop struct {
	accounts          accountFinder
	movements         movementFinder
	subcategories     subcategoryFinder
	chatHistory       chatHistory
	actions           actionQueue
	metrics           metricLog
	model             loopModel
	engine            *conversation.Engine
	writer            movementWriter
	resolveMetric     func(ctx context.Context, userID uint64, outcome string, movementIDs ...uint)
	handleGroqError   func(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error)
	enqueueUpdatePick func(ctx context.Context, chat messenger.Chat, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool
	answerQuery       func(ctx context.Context, chat messenger.Chat, userID uint64, text string) error
	manageSettings    func(ctx context.Context, chat messenger.Chat, userID uint64, text, area string) error
}

func New(
	accounts accountFinder,
	movements movementFinder,
	subcategories subcategoryFinder,
	chatHistory chatHistory,
	actions actionQueue,
	metrics metricLog,
	model loopModel,
	engine *conversation.Engine,
	writer movementWriter,
	resolveMetric func(ctx context.Context, userID uint64, outcome string, movementIDs ...uint),
	handleGroqError func(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error),
	enqueueUpdatePick func(ctx context.Context, chat messenger.Chat, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool,
	answerQuery func(ctx context.Context, chat messenger.Chat, userID uint64, text string) error,
	manageSettings func(ctx context.Context, chat messenger.Chat, userID uint64, text, area string) error,
) *Loop {
	return &Loop{
		accounts:          accounts,
		movements:         movements,
		subcategories:     subcategories,
		chatHistory:       chatHistory,
		actions:           actions,
		metrics:           metrics,
		model:             model,
		engine:            engine,
		writer:            writer,
		resolveMetric:     resolveMetric,
		handleGroqError:   handleGroqError,
		enqueueUpdatePick: enqueueUpdatePick,
		answerQuery:       answerQuery,
		manageSettings:    manageSettings,
	}
}

func (l *Loop) insertAction(action *pendingaction.PendingAction) error {
	if l.actions == nil {
		return nil
	}
	return l.actions.Insert(action)
}

const (
	SettingsAreaAccount        = "cuenta"
	SettingsAreaCategory       = "categoria"
	SettingsAreaCategoryManage = "categoria_administrar"
	SettingsAreaReminder       = "recordatorio"
)

const msgHelp = "Conmigo es fácil, me hablás normal:\n\n" +
	"📝 Anotar: «gasté 500 en el súper», «me pagaron 10 mil»\n" +
	"✏️ Corregir: «el súper eran 600»\n" +
	"🗑️ Borrar: «borrá el último gasto»\n" +
	"🔄 Transferir: «pasé 50 mil del banco a MP»\n" +
	"❓ Preguntar: «¿cuánto gasté esta semana?»\n" +
	"🗂️ Categorías: «creá una categoría para mascotas», «sacá la que repetí»\n" +
	"🏦 Cuentas y recordatorios: pedímelos cuando quieras."

const MsgAskRewrite = "✍️ Dale, mandalo de nuevo con más detalle (monto, categoría, y si es un movimiento nuevo)."
