package miniapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/user"
)

const (
	initDataMaxAge   = 24 * time.Hour
	initDataHeader   = "X-Telegram-Init-Data"
	contextUserIDKey = "miniapp_user_id"
)

// telegramInitDataUser is the subset of Telegram's `user` JSON field this
// package needs — just enough to resolve our own user_id.
type telegramInitDataUser struct {
	ID int64 `json:"id"`
}

// verifyInitData validates a Telegram Mini App initData string per
// https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app:
// HMAC-SHA256 the sorted "key=value" lines (excluding hash) with a secret
// derived from the bot token, compare in constant time, and reject stale
// auth_date. Returns the Telegram user id (as a string, matching
// user.User.TelegramID's type) on success.
func verifyInitData(initData, botToken string, now time.Time) (telegramID string, ok bool) {
	values, err := url.ParseQuery(initData)
	if err != nil {
		return "", false
	}

	receivedHash := values.Get("hash")
	if receivedHash == "" {
		return "", false
	}
	values.Del("hash")

	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+"="+values.Get(k))
	}
	dataCheckString := strings.Join(lines, "\n")

	secretKey := hmac.New(sha256.New, []byte("WebAppData"))
	secretKey.Write([]byte(botToken))
	secret := secretKey.Sum(nil)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(dataCheckString))
	expectedHash := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(expectedHash), []byte(receivedHash)) != 1 {
		return "", false
	}

	authDateUnix, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return "", false
	}
	authDate := time.Unix(authDateUnix, 0)
	if now.Sub(authDate) > initDataMaxAge {
		return "", false
	}

	var u telegramInitDataUser
	if err := json.Unmarshal([]byte(values.Get("user")), &u); err != nil {
		return "", false
	}
	if u.ID == 0 {
		return "", false
	}

	return strconv.FormatInt(u.ID, 10), true
}

// userLookup is the local interface authInitData needs — repo convention,
// mirrored by movementRepository/accountRepository in controller.go.
type userLookup interface {
	FindByTelegramID(telegramID string) (*user.User, error)
}

// authInitData is Gin middleware guarding every /app route. It reads
// initData from the X-Telegram-Init-Data header (htmx requests) or the
// "tgWebAppData" query param (first page load, before any JS runs), verifies
// it, resolves the Telegram user to our internal user_id, and stores it in
// the Gin context under contextUserIDKey. Aborts with 401 on any failure.
func authInitData(botToken string, users userLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		initData := c.GetHeader(initDataHeader)
		if initData == "" {
			initData = c.Query("tgWebAppData")
		}

		telegramID, ok := verifyInitData(initData, botToken, time.Now())
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		u, err := users.FindByTelegramID(telegramID)
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Set(contextUserIDKey, u.ID)
		c.Next()
	}
}
