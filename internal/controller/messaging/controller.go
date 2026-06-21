package messaging

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/invitation"
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

type controller struct {
	users       userRepository
	invitations invitationRepository
	engine      *conversation.Engine
}

func NewController(
	users userRepository,
	invitations invitationRepository,
	engine *conversation.Engine,
) *controller {
	return &controller{
		users:       users,
		invitations: invitations,
		engine:      engine,
	}
}

func (c *controller) RegisterHandlers(b *bot.Bot) {
	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, c.handleStart)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/cuentas", bot.MatchTypeExact, c.handleCuentasCommand)
	b.RegisterHandler(bot.HandlerTypeMessageText, "/subcategorias", bot.MatchTypeExact, c.handleSubcategoriesCommand)
	// Catch-all: cualquier texto libre (no comando) o callback se intenta
	// despachar al motor de conversaciones, si el usuario tiene un flujo
	// activo. Si no tiene ninguno, Handle devuelve found=false y no se
	// hace nada — no es turno de este handler responder.
	b.RegisterHandlerMatchFunc(c.hasIncomingInput, c.handleConversationInput)
}

func (c *controller) handleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	telegramID := fmt.Sprint(update.Message.From.ID)
	code := extractStartCode(update.Message.Text)

	if existing, err := c.users.FindByTelegramID(telegramID); err == nil {
		c.reply(ctx, b, update, msgAlreadyHasAccount)
		c.resumeOrStartAccountFlow(ctx, b, update.Message.Chat.ID, existing.ID)
		return
	}

	if code == "" {
		c.reply(ctx, b, update, msgPrivateBot)
		return
	}

	inv, err := c.invitations.FindByCode(code)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.reply(ctx, b, update, msgInvalidInvitation)
		return
	}
	if err != nil {
		c.reply(ctx, b, update, msgInvitationError)
		return
	}
	if inv.UsedAt != nil {
		c.reply(ctx, b, update, msgInvitationUsed)
		return
	}
	if time.Now().After(inv.ExpiresAt) {
		c.reply(ctx, b, update, msgInvitationExpired)
		return
	}

	newUser := &user.User{
		TelegramID: telegramID,
		Username:   update.Message.From.Username,
		IsAdmin:    false,
	}
	if err := c.users.Insert(newUser); err != nil {
		c.reply(ctx, b, update, msgUserCreationError)
		return
	}
	if err := c.invitations.MarkAsUsed(inv.ID, newUser.ID); err != nil {
		_ = err // no bloqueante, el user ya se creó
	}

	c.resumeOrStartAccountFlow(ctx, b, update.Message.Chat.ID, newUser.ID)
}

func (c *controller) handleCuentasCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	telegramID := fmt.Sprint(update.Message.From.ID)
	u, err := c.users.FindByTelegramID(telegramID)
	if err != nil {
		c.reply(ctx, b, update, msgPrivateBot)
		return
	}
	c.resumeOrStartAccountFlow(ctx, b, update.Message.Chat.ID, u.ID)
}

func (c *controller) handleSubcategoriesCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	telegramID := fmt.Sprint(update.Message.From.ID)
	u, err := c.users.FindByTelegramID(telegramID)
	if err != nil {
		c.reply(ctx, b, update, msgPrivateBot)
		return
	}
	c.startFlowIfNotBusy(ctx, b, update.Message.Chat.ID, u.ID, subcategory.FlowName)
}

// resumeOrStartAccountFlow arranca el flujo de alta de cuentas si el
// usuario no tiene ya uno en curso (de cualquier tipo). El Flow ya está
// registrado en el Engine desde que arrancó el server.
func (c *controller) resumeOrStartAccountFlow(ctx context.Context, b *bot.Bot, chatID int64, userID uint64) {
	c.startFlowIfNotBusy(ctx, b, chatID, userID, account.FlowName)
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
		return
	}
	if result.Finished {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: c.flowFinishedMessage(result.FlowName)})
		return
	}
	c.sendPrompt(ctx, b, chatID, result.Prompt)
}

// flowFinishedMessage decide qué mensaje de cierre mostrar según qué
// Flow terminó. Agregar un Flow nuevo solo necesita sumar un case acá.
func (c *controller) flowFinishedMessage(flowName string) string {
	switch flowName {
	case subcategory.FlowName:
		return msgSubcategorySetupFinished
	default:
		return msgAccountSetupFinished
	}
}

// sendPrompt traduce un conversation.Prompt neutro al formato real de
// Telegram (botones inline).
func (c *controller) sendPrompt(ctx context.Context, b *bot.Bot, chatID int64, prompt conversation.Prompt) {
	params := &bot.SendMessageParams{ChatID: chatID, Text: prompt.Text}

	if len(prompt.Buttons) > 0 {
		var row []models.InlineKeyboardButton
		for _, btn := range prompt.Buttons {
			row = append(row, models.InlineKeyboardButton{Text: btn.Label, CallbackData: btn.Data})
		}
		params.ReplyMarkup = &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{row},
		}
	}

	b.SendMessage(ctx, params)
}

func (c *controller) reply(ctx context.Context, b *bot.Bot, update *models.Update, text string) {
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: text})
}

func extractStartCode(text string) string {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
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
