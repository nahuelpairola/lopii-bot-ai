package admin

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-telegram/bot"
	messagingctrl "lopiibot.com/internal/controller/messaging"
	"lopiibot.com/internal/middleware"
	"lopiibot.com/internal/user"
)

type userReader interface {
	FindByChannel(channel, channelUserID string) (*user.User, error)
}
type resetter interface {
	SoftDeleteByUserID(userID uint64) error
}
type onboardingEngine interface {
	Clear(userID uint64) error
}

type controller struct {
	users     userReader
	accounts  resetter
	movements resetter
	engine    onboardingEngine
	bot       *bot.Bot
}

func NewController(users userReader, accounts, movements resetter, engine onboardingEngine, b *bot.Bot) *controller {
	return &controller{users: users, accounts: accounts, movements: movements, engine: engine, bot: b}
}

func (c *controller) RegisterRoutes(engine *gin.Engine) {
	// TODO: hardcoded adminID=1 hasta que exista login real (CLAUDE.md §6).
	engine.POST("/admin/users/:telegramID/reset", middleware.RequireAdmin(1), c.Reset)
}

// Reset soft-deletes the user's accounts+movements, clears their flow state,
// and re-fires onboarding (pushing the first prompt itself, so no next
// message is silently eaten). intent_events is deliberately untouched.
func (c *controller) Reset(ctx *gin.Context) {
	telegramID := ctx.Param("telegramID")
	u, err := c.users.FindByChannel(user.ChannelTelegram, telegramID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err := c.movements.SoftDeleteByUserID(u.ID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "could not reset movements"})
		return
	}
	if err := c.accounts.SoftDeleteByUserID(u.ID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "could not reset accounts"})
		return
	}
	if err := c.engine.Clear(u.ID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "could not clear flow state"})
		return
	}
	if c.bot != nil {
		if chatID, convErr := strconv.ParseInt(telegramID, 10, 64); convErr == nil {
			c.bot.SendMessage(context.Background(), &bot.SendMessageParams{ChatID: chatID, Text: messagingctrl.MsgAccountReset})
			c.bot.SendMessage(context.Background(), &bot.SendMessageParams{ChatID: chatID, Text: messagingctrl.MsgWelcome})
		}
	}
	ctx.JSON(http.StatusOK, gin.H{"status": "reset", "telegram_id": telegramID})
}
