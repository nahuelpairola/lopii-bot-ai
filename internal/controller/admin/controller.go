package admin

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	messagingctrl "lopiibot.com/internal/controller/messaging"
	"lopiibot.com/internal/messenger"
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

type chatResolver interface {
	ChatFor(userID uint64) (messenger.Chat, error)
}

type controller struct {
	users     userReader
	accounts  resetter
	movements resetter
	engine    onboardingEngine
	chats     chatResolver
}

func NewController(users userReader, accounts, movements resetter, engine onboardingEngine, chats chatResolver) *controller {
	return &controller{users: users, accounts: accounts, movements: movements, engine: engine, chats: chats}
}

func (c *controller) RegisterRoutes(engine *gin.Engine) {
	engine.POST("/admin/users/:telegramID/reset", middleware.RequireAdmin(1), c.Reset)
}

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
	if chat, chatErr := c.chats.ChatFor(u.ID); chatErr == nil {
		_ = messenger.SendText(context.Background(), chat, messagingctrl.MsgAccountReset)
		_ = messenger.SendText(context.Background(), chat, messagingctrl.MsgWelcome)
	}
	ctx.JSON(http.StatusOK, gin.H{"status": "reset", "telegram_id": telegramID})
}
