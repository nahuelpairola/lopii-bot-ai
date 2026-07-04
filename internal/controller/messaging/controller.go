package messaging

import (
	"context"
	"fmt"
	"strings"
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
}

type movementRepository interface {
	InsertBatch([]movement.Movement) error
	SumAmountForAccount(accountID uint64) (decimal.Decimal, error)
	ReplaceMovements(oldIDs []uint, newMovements []movement.Movement) error
	FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]movement.Movement, error)
	SoftDeleteByIDs(ids []uint) error
}

type subcategoryRepository interface {
	FindByCategoryAndSubcategory(category, subcategory string) (*subcategory.Subcategory, error)
	FindAllForUser(userID uint64) ([]subcategory.Subcategory, error)
	DistinctCategoriesForUser(userID uint64) ([]string, error)
}

// movementOrchestrator is the local interface for orchestrator.Orchestrator
// — only the methods this package's flows need.
type movementOrchestrator interface {
	ClassifyIntent(ctx context.Context, text string) (orchestrator.IntentResult, error)
	ClassifyCreate(ctx context.Context, text string, taxonomy []orchestrator.TaxonomyEntry, accounts []orchestrator.AccountOption, today string) (orchestrator.CreateResult, error)
	ResolveUpdate(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.UpdateResult, error)
	ResolveDelete(ctx context.Context, text string, candidate orchestrator.MovementCandidate) (orchestrator.DeleteResult, error)
}

type controller struct {
	users         userRepository
	invitations   invitationRepository
	accounts      accountRepository
	movements     movementRepository
	subcategories subcategoryRepository
	engine        *conversation.Engine
	orchestrator  movementOrchestrator
}

func NewController(
	users userRepository,
	invitations invitationRepository,
	accounts accountRepository,
	movements movementRepository,
	subcategories subcategoryRepository,
	engine *conversation.Engine,
	orch movementOrchestrator,
) *controller {
	return &controller{
		users:         users,
		invitations:   invitations,
		accounts:      accounts,
		movements:     movements,
		subcategories: subcategories,
		engine:        engine,
		orchestrator:  orch,
	}
}

func (c *controller) RegisterHandlers(b *bot.Bot) {
	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, c.handleStart)
	b.RegisterHandlerMatchFunc(c.hasIncomingInput, c.handleConversationInput)
}

// startFlowIfNotBusy arranca cualquier Flow ya registrado en el Engine
// para un usuario, salvo que ya tenga uno en curso (de cualquier tipo).
// Este es el único lugar que conoce el mecanismo de "no pisar un flujo
// activo" — agregar un comando nuevo que arranque otro Flow (como
// /subcategorias) solo necesita llamar a este método con el nombre
// correspondiente, sin duplicar la lógica.
func (c *controller) startFlowIfNotBusy(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, flowName string) {
	if inProgress, err := c.engine.InProgress(userID); err == nil && inProgress {
		return
	}
	prompt, err := c.engine.Start(userID, flowName)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
		return
	}
	c.sendPrompt(ctx, b, chatID, prompt)
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
	telegramID := updateTelegramID(update)
	if telegramID == "" {
		return
	}
	u, err := c.users.FindByTelegramID(telegramID)
	if err != nil {
		return
	}

	if cb := update.CallbackQuery; cb != nil {
		b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})
	}

	input := toConversationInput(update)
	chatID := updateChatID(update)

	result, found, err := c.engine.Handle(u.ID, input)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
		return
	}
	if !found {
		if input.Text != "" {
			c.handleFreeText(ctx, b, chatID, u.ID, input.Text)
		}
		return
	}
	if result.Finished {
		c.handleFlowFinished(ctx, b, chatID, result)
		return
	}
	c.sendPrompt(ctx, b, chatID, result.Prompt)
}

// handleFlowFinished ejecuta la acción real correspondiente a un flow que
// acaba de terminar (crear cuenta, insertar movimiento, etc.), según su
// nombre. Agregar un flow nuevo implica agregar un case acá.
func (c *controller) handleFlowFinished(ctx context.Context, b *bot.Bot, chatID int64, result conversation.Result) {
	switch result.FlowName {
	case initialBalanceFlowName:
		c.finishInitialBalanceFlow(ctx, b, chatID, result.Data)
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
	params := &bot.SendMessageParams{ChatID: chatID, Text: prompt.Text, ParseMode: models.ParseModeHTML}

	if rows := chunkButtons(prompt.Buttons); len(rows) > 0 {
		params.ReplyMarkup = &models.InlineKeyboardMarkup{InlineKeyboard: rows}
	}

	b.SendMessage(ctx, params)
}

func (c *controller) reply(ctx context.Context, b *bot.Bot, update *models.Update, text string) {
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
