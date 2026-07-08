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

func (c *controller) handleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	telegramID := fmt.Sprint(update.Message.From.ID)
	code := extractStartCode(update.Message.Text)

	if _, err := c.users.FindByTelegramID(telegramID); err == nil {
		c.reply(ctx, b, update, msgAlreadyHasAccount)
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
		_ = err
	}

	c.reply(ctx, b, update, msgUserCreatedSuccessfully)
	c.startFlowIfNotBusy(ctx, b, update.Message.Chat.ID, newUser.ID, OnboardingCollectFlowName)
}

func extractStartCode(text string) string {
	parts := strings.Fields(text)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}
