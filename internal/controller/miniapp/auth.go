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

// userLookup is the local interface authInitData needs — repo convention,
// mirrored by movementReader/accountReader in controller.go.
type userLookup interface {
	FindByTelegramID(telegramID string) (*user.User, error)
}

// authInitData is Gin middleware for the /app view routes. It enforces auth
// ONLY on htmx requests (which carry initData in the X-Telegram-Init-Data
// header, injected client-side by app.js). A plain full-page navigation can
// never carry initData — Telegram delivers it only to client JS via the
// launch-URL hash — so those requests pass through and the handler serves the
// unauthenticated shell, whose htmx content-load THEN authenticates. Data is
// therefore only ever rendered behind a verified initData (partial path).
func authInitData(botToken string, users userLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader(hxRequestHeader) == "" {
			// full-page nav: no initData yet. Serve the shell, which self-loads
			// its content via htmx (that request IS authenticated).
			c.Status(http.StatusOK)
			templates.Shell(activeFromPath(c.Request.URL.Path), c.Request.URL.Path).
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

		u, err := users.FindByTelegramID(strconv.FormatInt(parsed.User.ID, 10))
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Set(contextUserIDKey, u.ID)
		c.Next()
	}
}

// activeFromPath returns the tab key for a request path — the first segment
// after "/app/" (e.g. "/app/categorias/Comida" → "categorias"). The tabbar
// highlights the tab whose key matches.
func activeFromPath(path string) string {
	rest := strings.TrimPrefix(path, "/app/")
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return rest[:i]
	}
	return rest
}
