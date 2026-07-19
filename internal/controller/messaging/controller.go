package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/queryhistory"
	"lopiibot.com/internal/reminder"
	"lopiibot.com/internal/subcategory"
	"lopiibot.com/internal/user"
)

type userRepository interface {
	FindByTelegramID(telegramID string) (*user.User, error)
	Insert(u *user.User) error
}

type invitationRepository interface {
	FindByCode(code string) (*invitation.Invitation, error)
	MarkAsUsed(id uint64, userID uint64) error
}

type accountRepository interface {
	Insert(*account.Account) error
	FindDefaultByCurrency(userID uint64, currency currency.Currency) (*account.Account, error)
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
	FindRecentlyCreatedForUser(userID uint64, since time.Time) ([]movement.Movement, error)
	SoftDeleteByIDs(ids []uint) error
	InsertAccountsWithOpenings(items []movement.AccountOpening) error
	SumForUser(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error)
	ListForUser(q movement.MovementQuery, limit int) ([]movement.Movement, error)
	ReassignAccount(fromID, toID uint64) error
}

type subcategoryRepository interface {
	FindByCategoryAndSubcategory(userID uint64, category, subcategory string) (*subcategory.Subcategory, error)
	FindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	DistinctCategoriesForUser(userID uint64) ([]string, error)
	IconForCategory(userID uint64, category string) string
	Insert(s *subcategory.Subcategory) error
	Reload() error
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
	}
}

func (c *controller) RegisterHandlers(b *bot.Bot) {
	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, c.handleStart)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, constants.WeeklySummaryOffData, bot.MatchTypeExact, c.handleWeeklySummaryOff)
	b.RegisterHandlerMatchFunc(c.hasIncomingInput, c.handleConversationInput)
}

// hasIncomingInput matchea cualquier mensaje de texto (que no sea
// comando) o callback de botón — son los únicos tipos de update que el
// motor de conversaciones puede llegar a procesar.
func (c *controller) hasIncomingInput(update *models.Update) bool {
	if update.CallbackQuery != nil {
		return update.CallbackQuery.Data != constants.WeeklySummaryOffData
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
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
			return &uid, err
		}
		if !found {
			if input.Text != "" {
				return &uid, c.handleFreeText(ctx, b, chatID, u.ID, input.Text)
			}
			return &uid, nil
		}
		if result.Finished {
			c.handleFlowFinished(ctx, b, chatID, result)
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
	case movementConfirmFlowName:
		c.finishMovementConfirmFlow(ctx, b, chatID, result.Data)
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
	case movementNegativeConfirmFlowName:
		c.finishMovementNegativeConfirmFlow(ctx, b, chatID, result.Data)
	case reminderSetupFlowName:
		c.finishReminderSetup(ctx, b, chatID, result.Data)
	default:
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
	}
}

// buttonsPerRow caps how many inline-keyboard buttons Telegram renders
// per row — putting every option in a single row (the old behavior) is
// what made category/subcategory/account buttons unreadably small.
const buttonsPerRow = 2

func chunkButtons(buttons []conversation.Button) [][]models.InlineKeyboardButton {
	if len(buttons) == 0 {
		return nil
	}
	rows := make([][]models.InlineKeyboardButton, 0, (len(buttons)+buttonsPerRow-1)/buttonsPerRow)
	for i := 0; i < len(buttons); i += buttonsPerRow {
		end := i + buttonsPerRow
		if end > len(buttons) {
			end = len(buttons)
		}
		row := make([]models.InlineKeyboardButton, 0, end-i)
		for _, btn := range buttons[i:end] {
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
