package miniapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	testBotToken    = "123456:TEST-TOKEN"
	testBotUsername = "testbot"
)

func timeNow() time.Time { return time.Now() }

func buildInitData(t *testing.T, telegramID string, authDate time.Time, botToken string) string {
	t.Helper()
	values := url.Values{}
	values.Set("user", `{"id":`+telegramID+`,"first_name":"Test"}`)
	values.Set("auth_date", strconv.FormatInt(authDate.Unix(), 10))
	values.Set("query_id", "AAABBBCCC")

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
	values.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return values.Encode()
}

func authedHTMXRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("X-Telegram-Init-Data", buildInitData(t, "999", timeNow(), testBotToken))
	return req
}
