package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/queryhistory"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

type userRepository interface {
	FindByTelegramID(telegramID string) (*user.User, error)
	FindByID(id uint64) (*user.User, error)
	Insert(u *user.User) error
}

type invitationRepository interface {
	FindByCode(code string) (*invitation.Invitation, error)
	MarkAsUsed(id uint64, userID uint64) error
}

type accountRepository interface {
	Insert(*account.Account) error
	FindDefaultByCurrency(userID uint64, currency currency.Currency) (*account.Account, error)
	HasDefaultForCurrency(userID uint64, currency currency.Currency) bool
	FindByUserID(userID uint64) ([]account.Account, error)
	GetAccount(id uint64) (*account.Account, error)
	Rename(accountID uint64, name string) error
	UnsetDefault(userID uint64, currency currency.Currency) error
	SetDefault(accountID uint64) error
}

type movementRepository interface {
	InsertBatch([]movement.Movement) error
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
	ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error
	FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error)
	FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]movement.Movement, error)
	SoftDeleteByIDs(ids []uint) error
	InsertAccountsWithOpenings(items []movement.AccountOpening) error
	SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	ListForUser(q movement.MovementQuery, limit int) ([]movement.Movement, error)
	ReassignAccount(fromID, toID uint64) error
	CountForUser(userID uint64) (int64, error)
	CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error)
	ReassignSubcategory(userID uint64, fromID uint64, toID uint64) error
	TopMerchantsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error)
}

type subcategoryRepository interface {
	FindByCategoryAndSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
	FindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	DistinctCategoriesForUser(userID uint64) ([]string, error)
	IconForCategory(userID uint64, category string) string
	Insert(s *subcategory.Subcategory) error
	Reload() error
	Delete(userID uint64, id uint64) error
	FindOwnedByUser(userID uint64) ([]subcategory.Subcategory, error)
}

// movementOrchestrator is the local interface for orchestrator.Orchestrator
// — only the methods this package's flows need.
type movementOrchestrator interface {
	ClassifyIntent(ctx context.Context, text string) (orchestrator.IntentResult, error)
	ClassifyCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry, accounts []orchestrator.AccountOption, today string) (orchestrator.CreateResult, error)
	ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate, accounts []orchestrator.AccountOption) (orchestrator.UpdateResult, error)
	ResolveDelete(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.DeleteResult, error)
	ClassifyOnboarding(ctx context.Context, text string) (orchestrator.OnboardingResult, error)
	ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry) (orchestrator.CategoryCreateResult, error)
	ResolveAccountManage(ctx context.Context, text string, accounts []orchestrator.AccountOption) (orchestrator.AccountManageResult, error)
	AnswerQuery(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error)
	// Run is the unified agent loop. Added in stage 1 and called by nothing
	// yet: stage 2 routes UPDATE/DELETE through it. The interface deliberately
	// grows before it shrinks (9 → 3 in stage 5) — that is what lets each
	// stage be bisected on its own.
	Run(ctx context.Context, systemPrompt, userText string, history []orchestrator.QueryTurn, tools []orchestrator.AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error)
}

type metricRepository interface {
	Log(userID uint64, traceID, rawMessage, intent string, needsConfirmation bool, outcome string) error
	Resolve(userID uint64, outcome string, movementIDs []uint) error
}

type queryHistoryRepository interface {
	Recent(userID uint64) ([]queryhistory.Turn, error)
	Append(userID uint64, question, answer string) error
}

type reminderRepository interface {
	Upsert(r *reminder.Reminder) error
	Disable(userID uint64) error
	FindByUserID(userID uint64) (*reminder.Reminder, error)
	SetWeeklySummary(userID uint64, enabled bool) error
}

type traceRepository interface {
	InsertRequestTrace(traceID string, userID *uint64, updateType string, receivedAt time.Time, latencyMs int, errMsg string) error
}

// nudgeRepository is the once-ever/cooldown storage for contextual nudges
// (internal/nudge). Local interface — see nudge.go.
type nudgeRepository interface {
	WasSent(userID uint64, key string) (bool, error)
	MarkSent(userID uint64, key string) error
	LastSentAt(userID uint64) (*time.Time, error)
}

// jobsRepository is the pending_llm_jobs storage (internal/pendingjob) —
// the durable cache for a user message that hit a terminal Groq 429.
type jobsRepository interface {
	Insert(job *pendingjob.PendingJob) error
	ListByUserOrdered(userID uint64) ([]pendingjob.PendingJob, error)
	ListPendingUserIDs() ([]uint64, error)
	Delete(id uint64) error
	CountByUser(userID uint64) (int64, error)
}

type controller struct {
	users         userRepository
	invitations   invitationRepository
	accounts      accountRepository
	movements     movementRepository
	subcategories subcategoryRepository
	engine        *conversation.Engine
	orchestrator  movementOrchestrator
	metrics       metricRepository
	queryHistory  queryHistoryRepository
	reminders     reminderRepository
	traces        traceRepository
	nudges        nudgeRepository
	jobs          jobsRepository
	nextDrainAt   time.Time
	drainMu       sync.Mutex
}

func NewController(
	users userRepository,
	invitations invitationRepository,
	accounts accountRepository,
	movements movementRepository,
	subcategories subcategoryRepository,
	engine *conversation.Engine,
	orch movementOrchestrator,
	metrics metricRepository,
	queryHistory queryHistoryRepository,
	reminders reminderRepository,
	traces traceRepository,
	nudges nudgeRepository,
	jobs jobsRepository,
) *controller {
	return &controller{
		users:         users,
		invitations:   invitations,
		accounts:      accounts,
		movements:     movements,
		subcategories: subcategories,
		engine:        engine,
		orchestrator:  orch,
		metrics:       metrics,
		queryHistory:  queryHistory,
		reminders:     reminders,
		traces:        traces,
		nudges:        nudges,
		jobs:          jobs,
	}
}

func (c *controller) RegisterHandlers(b *bot.Bot) {
	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, c.handleStart)
	b.RegisterHandlerMatchFunc(c.hasIncomingInput, c.handleConversationInput)
}

// hasIncomingInput matchea cualquier mensaje de texto (que no sea
// comando) o callback de botón — son los únicos tipos de update que el
// motor de conversaciones puede llegar a procesar.
func (c *controller) hasIncomingInput(update *models.Update) bool {
	if update.CallbackQuery != nil {
		return true
	}
	if update.Message != nil && update.Message.Text != "" && !strings.HasPrefix(update.Message.Text, "/") {
		return true
	}
	return false
}

// handleConversationInput le pasa el input al motor de conversaciones.
func (c *controller) handleConversationInput(ctx context.Context, b *bot.Bot, update *models.Update) {
	c.withTrace(ctx, update, func(ctx context.Context) (*uint64, error) {
		telegramID := updateTelegramID(update)
		if telegramID == "" {
			return nil, nil
		}
		u, err := c.users.FindByTelegramID(telegramID)
		if err != nil {
			return nil, err
		}
		uid := u.ID

		if cb := update.CallbackQuery; cb != nil {
			b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
		}

		input := toConversationInput(update)
		chatID := updateChatID(update)

		result, found, err := c.engine.Handle(u.ID, input)
		if err != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgSomethingBroke})
			return &uid, err
		}
		if !found {
			if input.Text != "" {
				if c.enqueueBehindPending(ctx, b, chatID, u.ID, input.Text) {
					return &uid, nil
				}
				err := c.handleFreeText(ctx, b, chatID, u.ID, input.Text)
				c.maybeNudge(ctx, b, chatID, u.ID)
				return &uid, err
			}
			return &uid, nil
		}
		if result.Finished {
			c.handleFlowFinished(ctx, b, chatID, result)
			c.maybeNudge(ctx, b, chatID, u.ID)
			return &uid, nil
		}
		c.sendPrompt(ctx, b, chatID, result.Prompt)
		return &uid, nil
	})
}

// handleFlowFinished ejecuta la acción real correspondiente a un flow que
// acaba de terminar (crear cuenta, insertar movimiento, etc.), según su
// nombre. Agregar un flow nuevo implica agregar un case acá.
func (c *controller) handleFlowFinished(ctx context.Context, b *bot.Bot, chatID int64, result conversation.Result) {
	slog.InfoContext(ctx, "flow finished", "flow", result.FlowName)
	if stringOrEmpty(result.Data[conversation.ResumeCancelledKey]) == "true" {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgResumeCancelled})
		return
	}
	switch result.FlowName {
	case movementCreateFlowName:
		c.finishMovementCreateFlow(ctx, b, chatID, result.Data)
	case movementUpdatePickFlowName:
		c.finishMovementUpdatePickFlow(ctx, b, chatID, result.Data)
	case movementUpdateConfirmFlowName:
		c.finishMovementUpdateConfirmFlow(ctx, b, chatID, result.Data)
	case movementDeleteFlowName:
		c.finishMovementDeleteFlow(ctx, b, chatID, result.Data)
	case accountCreateFlowName:
		c.finishAccountCreateFlow(ctx, b, chatID, result.Data)
	case accountManageFlowName:
		c.finishAccountManageFlow(ctx, b, chatID, result.Data)
	case accountMoveOfferFlowName:
		c.finishAccountMoveOffer(ctx, b, chatID, result.Data)
	case subcategorySetupFlowName:
		c.finishSubcategorySetupFlow(ctx, b, chatID, result.Data)
	case categoryMatchOfferFlowName:
		c.finishCategoryMatchOffer(ctx, b, chatID, result.Data)
	case categoryProposalConfirmFlowName:
		c.finishCategoryProposalConfirm(ctx, b, chatID, result.Data)
	case categoryManagePickFlowName:
		c.finishCategoryManagePickFlow(ctx, b, chatID, result.Data)
	case categoryManageTargetFlowName:
		c.finishCategoryManageTargetFlow(ctx, b, chatID, result.Data)
	case movementNegativeConfirmFlowName:
		c.finishMovementNegativeConfirmFlow(ctx, b, chatID, result.Data)
	case reminderSetupFlowName:
		c.finishReminderSetup(ctx, b, chatID, result.Data)
	default:
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgSomethingBroke})
	}
}

// buttonsPerRow caps how many inline-keyboard buttons Telegram renders
// per row — putting every option in a single row (the old behavior) is
// what made category/subcategory/account buttons unreadably small.
const buttonsPerRow = 2

func chunkButtons(buttons []conversation.Button) [][]models.InlineKeyboardButton {
	var rows [][]models.InlineKeyboardButton
	for chunk := range slices.Chunk(buttons, buttonsPerRow) {
		row := make([]models.InlineKeyboardButton, 0, len(chunk))
		for _, btn := range chunk {
			row = append(row, models.InlineKeyboardButton{Text: btn.Label, CallbackData: btn.Data})
		}
		rows = append(rows, row)
	}
	return rows
}

// sendPrompt traduce un conversation.Prompt neutro al formato real de
// Telegram (botones inline, en grilla de buttonsPerRow por fila).
func (c *controller) sendPrompt(ctx context.Context, b *bot.Bot, chatID int64, prompt conversation.Prompt) {
	if b == nil {
		return
	}
	params := &bot.SendMessageParams{ChatID: chatID, Text: prompt.Text, ParseMode: models.ParseModeHTML}

	if rows := chunkButtons(prompt.Buttons); len(rows) > 0 {
		params.ReplyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: rows}
	}

	b.SendMessage(ctx, params)
}

// startFlow arranca un flow sembrado y manda su primer prompt. Absorbe el bloque
// que se repetía en los sitios que resuelven un fallo de arranque de la misma
// forma: avisarle al usuario y devolver el error envuelto.
//
// errCtx es el prefijo del error. No es cosmético: ese string sube hasta
// withTrace y termina en la columna request_traces.error, así que es lo único
// que distingue "no arrancó el flujo de cuentas" de "no arrancó el de
// movimientos" cuando se mira la traza después.
//
// Los call sites que fallan distinto (los que caen al wizard, los que no
// devuelven error, los que no le avisan al usuario) NO usan este helper — meter
// esas variantes acá pediría un callback por caso y sería más código, no menos.
func (c *controller) startFlow(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, flowName string, seed conversation.Data, errCtx string) error {
	prompt, err := c.engine.StartWithData(userID, flowName, seed)
	if err != nil {
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return fmt.Errorf("%s: %w", errCtx, err)
	}
	c.sendPrompt(ctx, b, chatID, prompt)
	return nil
}

func (c *controller) reply(ctx context.Context, b *bot.Bot, update *models.Update, text string) {
	if b == nil {
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: text})
}

func toConversationInput(update *models.Update) conversation.Input {
	if update.CallbackQuery != nil {
		return conversation.Input{CallbackData: update.CallbackQuery.Data}
	}
	return conversation.Input{Text: update.Message.Text}
}

func updateTelegramID(update *models.Update) string {
	if update.Message != nil && update.Message.From != nil {
		return fmt.Sprint(update.Message.From.ID)
	}
	if update.CallbackQuery != nil {
		return fmt.Sprint(update.CallbackQuery.From.ID)
	}
	return ""
}

func updateChatID(update *models.Update) int64 {
	if update.Message != nil {
		return update.Message.Chat.ID
	}
	if update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil {
		return update.CallbackQuery.Message.Message.Chat.ID
	}
	return 0
}
