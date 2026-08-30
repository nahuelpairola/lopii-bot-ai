package miniapp

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	initdata "github.com/telegram-mini-apps/init-data-golang"
	"lopiibot.com/internal/controller/miniapp/templates"
	"lopiibot.com/internal/user"
)

const (
	initDataMaxAge   = 24 * time.Hour
	initDataHeader   = "X-Telegram-Init-Data"
	contextUserIDKey = "miniapp_user_id"
)

type userLookup interface {
	FindByChannel(channel, channelUserID string) (*user.User, error)
}

func authInitData(botToken string, users userLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader(hxRequestHeader) == "" {

			loadPath := c.Request.URL.Path
			if q := c.Request.URL.RawQuery; q != "" {
				loadPath += "?" + q
			}
			c.Status(http.StatusOK)
			templates.Shell(activeFromPath(c.Request.URL.Path), loadPath).
				Render(c.Request.Context(), c.Writer)
			c.Abort()
			return
		}

		raw := c.GetHeader(initDataHeader)
		if err := initdata.Validate(raw, botToken, initDataMaxAge); err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		parsed, err := initdata.Parse(raw)
		if err != nil || parsed.User.ID == 0 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		u, err := users.FindByChannel(user.ChannelTelegram, strconv.FormatInt(parsed.User.ID, 10))
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Set(contextUserIDKey, u.ID)
		c.Next()
	}
}

func activeFromPath(path string) string {
	rest := strings.TrimPrefix(path, templates.AppPrefix+"/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}
