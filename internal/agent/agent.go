// Package agent es el loop unificado que resuelve un mensaje de texto libre:
// elige herramienta, resuelve referencias, parkea acciones, y delega los gates
// de escritura a flow. Vive en su propio paquete (extraído de messaging en la
// costura de la etapa 6) y NO sabe nada de Telegram-webhook ni del controller.
//
// Lo que el loop necesita del mundo exterior es la interfaz agentServices, que
// el borde (controller/messaging) implementa. Un services es la unión de los
// repos de lectura/escritura y los outbounds: sendText, sendPrompt, startFlow,
// y los bridges a flow (ResolveAndInsertMovements, MaybeNearDuplicate) y a los
// loops vecinos (FinishAnswerQuery, FinishManageSettings).
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

// agentServices es lo que el loop necesita del mundo. Métodos exportados porque
// una interfaz con métodos unexported solo la implementan tipos del mismo
// paquete; el controller la implementa con puentes de una línea.
type agentServices interface {
	// Repos de lectura.
	FindUserAccounts(userID uint64) ([]account.Account, error)
	AccountsHasDefaultForCurrency(userID uint64, cur currency.Currency) bool
	FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error)
	MovementsFindSimilarForUser(userID uint64, message string, since time.Time, until *time.Time) ([]movement.Movement, error)
	SubcategoriesFindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	FindSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
	ChatHistoryRecent(userID uint64) ([]chathistory.Turn, error)
	ChatHistoryAppend(userID uint64, question, answer string) error

	// Cola de acciones parkeadas.
	ActionsInsert(a *pendingaction.PendingAction) error
	ActionsNextForUser(userID uint64) (*pendingaction.PendingAction, error)
	ActionsUpdate(a *pendingaction.PendingAction) error
	ActionsDelete(id uint64) error
	ActionsEnabled() bool

	// Métricas (intent_events).
	MetricsLog(userID uint64, traceID, rawMessage, intent string, needsConfirmation bool, outcome string) error
	ResolveMetric(ctx context.Context, userID uint64, outcome string, movementIDs ...uint)
	MetricsSetIntentIfQueued(userID uint64, intent string) error

	// El orquestador (Groq).
	Run(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error)
	ClassifyCategories(ctx context.Context, message string, rows []orchestrator.ClassifyRow, taxonomy []orchestrator.TaxonomyEntry) []orchestrator.Pair
	ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error)

	// Outbounds al canal y a los flows.
	SendText(ctx context.Context, chat messenger.Chat, text string)
	SendPrompt(ctx context.Context, chat messenger.Chat, prompt conversation.Prompt)
	StartFlow(ctx context.Context, chat messenger.Chat, userID uint64, flowName string, seed conversation.Data, errCtx string) error
	EngineStartWithData(userID uint64, flowName string, seed conversation.Data) (conversation.Prompt, error)
	IsReplaying(ctx context.Context) bool

	// Bridges al gate de casi-duplicado y al pipeline de escritura.
	MaybeNearDuplicate(userID uint64, inserted []movement.Movement) []conversation.Button
	ResolveAndInsertMovements(data conversation.Data) ([]movement.Movement, error)

	// El 429 y los loops vecinos.
	HandleGroqError(ctx context.Context, chat messenger.Chat, userID uint64, text string, err error) (bool, error)
	EnqueueUpdatePickIfRateLimited(ctx context.Context, chat messenger.Chat, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, err error) bool
	FinishAnswerQuery(ctx context.Context, chat messenger.Chat, userID uint64, text string) error
	FinishManageSettings(ctx context.Context, chat messenger.Chat, userID uint64, text, area string) error
}

// StartLoop resuelve un mensaje con el loop unificado. Es el único camino de un
// texto libre: no hay router que filtre antes.
func StartLoop(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, text string) error {
	return startAgentLoop(ctx, svc, chat, userID, text)
}

// FinishAskUser corre cuando el usuario terminó de contestar el flujo de
// preguntas de una acción parkeada (ask_user).
func FinishAskUser(ctx context.Context, svc agentServices, chat messenger.Chat, data conversation.Data) {
	finishAskUserFlow(ctx, svc, chat, data)
}

// FinishMovementUpdatePick corre cuando el usuario eligió un candidato de la
// lista ambigua de una corrección.
func FinishMovementUpdatePick(ctx context.Context, svc agentServices, chat messenger.Chat, data conversation.Data) {
	finishMovementUpdatePickFlow(ctx, svc, chat, data)
}

// ProceedToUpdateConfirm resuelve el cambio del usuario contra el candidato ya
// encontrado (Camino 2 de UPDATE). Lo usa el drenaje al replayar un update_pick
// encolado por un 429.
func ProceedToUpdateConfirm(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, ask ChangeAsk) error {
	return proceedToUpdateConfirm(ctx, svc, chat, userID, message, transactionID, oldIDs, beforeRows, ask)
}

// DrainNextAction abre la próxima acción parkeada del usuario. Se llama al
// terminar un flujo y al final de un turno del loop que no abrió ninguno.
func DrainNextAction(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64) error {
	return drainNextAgentAction(ctx, svc, chat, userID)
}

// SettingsArea* son las áreas de manage_settings. Son el enum del schema: si
// divergen, el modelo manda un área que el switch de finishManageSettings no
// conoce y el pedido muere en ask_rewrite.
const (
	// SettingsAreaAccount es gestionar cuentas; SettingsAreaCategory es ALTA de
	// categoría; SettingsAreaCategoryManage es sacar o fusionar una que el
	// usuario ya creó. Son dos wizards distintos y ninguno sabe hacer lo del
	// otro, así que la distinción tiene que llegar desde el modelo — que la
	// tiene fácil: la dice el verbo.
	SettingsAreaAccount        = "cuenta"
	SettingsAreaCategory       = "categoria"
	SettingsAreaCategoryManage = "categoria_administrar"
	SettingsAreaReminder       = "recordatorio"
)
