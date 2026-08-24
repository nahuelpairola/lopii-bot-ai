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
	"lopiibot.com/internal/user"
)

// HandleStart es el handler de /start CODE — un canje de invitación por
// deep-link de Telegram, no un concepto neutro. Exportado porque la Task 9 lo
// cuelga con Transport.RegisterCommand, fuera de este paquete.
func (c *controller) HandleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	c.traced(ctx, "command", update.Message.Text, func(ctx context.Context) (*uint64, error) {
		telegramID := fmt.Sprint(update.Message.From.ID)
		code := extractStartCode(update.Message.Text)

		if existing, err := c.users.FindByChannel(user.ChannelTelegram, telegramID); err == nil {
			c.reply(ctx, b, update, msgAlreadyHasAccount)
			uid := existing.ID
			return &uid, nil
		}

		if code == "" {
			c.reply(ctx, b, update, msgPrivateBot)
			return nil, nil
		}

		inv, err := c.invitations.FindByCode(code)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.reply(ctx, b, update, msgInvalidInvitation)
			return nil, nil
		}
		if err != nil {
			c.reply(ctx, b, update, msgInvitationError)
			return nil, err
		}
		if inv.UsedAt != nil {
			c.reply(ctx, b, update, msgInvitationUsed)
			return nil, nil
		}
		if time.Now().After(inv.ExpiresAt) {
			c.reply(ctx, b, update, msgInvitationExpired)
			return nil, nil
		}

		newUser := &user.User{
			Username: update.Message.From.Username,
			IsAdmin:  false,
		}
		if err := c.users.InsertWithChannel(newUser, user.ChannelTelegram, telegramID); err != nil {
			c.reply(ctx, b, update, msgUserCreationError)
			return nil, err
		}
		if err := c.invitations.MarkAsUsed(inv.ID, newUser.ID); err != nil {
			_ = err
		}

		c.reply(ctx, b, update, msgWelcome)
		uid := newUser.ID
		return &uid, nil
	})
}

func extractStartCode(text string) string {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// reply es Telegram-shaped a propósito: el onboarding sigue siendo un
// deep-link de Telegram, no un concepto neutro (ver el comentario de
// HandleStart). Su único llamador es HandleStart.
func (c *controller) reply(ctx context.Context, b *bot.Bot, update *models.Update, text string) {
	if b == nil {
		return
	}
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: update.Message.Chat.ID, Text: text})
}
