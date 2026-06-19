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
	"lopiibot.com/internal/invitation"
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
}

func NewController(users userRepository, invitations invitationRepository) *controller {
	return &controller{users: users, invitations: invitations}
}

func (c *controller) RegisterHandlers(b *bot.Bot) {
	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, c.handleStart)
}

func (c *controller) handleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	telegramID := fmt.Sprint(update.Message.From.ID)
	code := extractStartCode(update.Message.Text)

	// Usuario ya existe, /start sin nada nuevo que hacer
	if _, err := c.users.FindByTelegramID(telegramID); err == nil {
		c.reply(ctx, b, update, "Ya tenés una cuenta activa. Mandame un gasto para registrarlo.")
		return
	}

	if code == "" {
		c.reply(ctx, b, update, "Este bot es privado. Si tenés una invitación, abrí el link que te compartieron.")
		return
	}

	inv, err := c.invitations.FindByCode(code)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.reply(ctx, b, update, "Esa invitación no es válida.")
		return
	}
	if err != nil {
		c.reply(ctx, b, update, "Hubo un error procesando tu invitación, probá de nuevo en un momento.")
		return
	}

	if inv.UsedAt != nil {
		c.reply(ctx, b, update, "Esa invitación ya fue utilizada.")
		return
	}
	if time.Now().After(inv.ExpiresAt) {
		c.reply(ctx, b, update, "Esa invitación expiró, pedí una nueva.")
		return
	}

	newUser := &user.User{
		TelegramID: telegramID,
		Username:   update.Message.From.Username,
		IsAdmin:    false,
		CreatedAt:  time.Now(),
	}
	if err := c.users.Insert(newUser); err != nil {
		c.reply(ctx, b, update, "No pude crear tu cuenta, probá de nuevo.")
		return
	}

	if err := c.invitations.MarkAsUsed(inv.ID, newUser.ID); err != nil {
		// el user ya se creó, no es bloqueante para el usuario
		_ = err
	}

	c.reply(ctx, b, update, "Bienvenido! Tu cuenta fue creada correctamente.")
}

func (c *controller) reply(ctx context.Context, b *bot.Bot, update *models.Update, text string) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   text,
	})
}

// extractStartCode parsea "/start XK9-2024" y devuelve "XK9-2024".
// Si no hay código, devuelve "".
func extractStartCode(text string) string {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}
