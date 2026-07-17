package miniapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testBotToken = "123456:TEST-TOKEN"

// buildInitData constructs a validly-signed initData string for a given
// telegram user id and auth_date, mirroring what Telegram's webview sends.
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
	hash := hex.EncodeToString(mac.Sum(nil))

	values.Set("hash", hash)
	return values.Encode()
}

func TestVerifyInitData_Valid(t *testing.T) {
	initData := buildInitData(t, "999", time.Now(), testBotToken)

	telegramID, ok := verifyInitData(initData, testBotToken, time.Now())

	if !ok {
		t.Fatal("expected ok=true for a validly-signed initData")
	}
	if telegramID != "999" {
		t.Fatalf("expected telegramID=999, got %q", telegramID)
	}
}

func TestVerifyInitData_BrokenHash(t *testing.T) {
	initData := buildInitData(t, "999", time.Now(), testBotToken)
	initData = strings.Replace(initData, "hash=", "hash=deadbeef", 1)

	_, ok := verifyInitData(initData, testBotToken, time.Now())

	if ok {
		t.Fatal("expected ok=false for a tampered hash")
	}
}

func TestVerifyInitData_Expired(t *testing.T) {
	old := time.Now().Add(-25 * time.Hour)
	initData := buildInitData(t, "999", old, testBotToken)

	_, ok := verifyInitData(initData, testBotToken, time.Now())

	if ok {
		t.Fatal("expected ok=false for auth_date older than 24h")
	}
}

func TestVerifyInitData_WrongToken(t *testing.T) {
	initData := buildInitData(t, "999", time.Now(), testBotToken)

	_, ok := verifyInitData(initData, "OTHER-TOKEN", time.Now())

	if ok {
		t.Fatal("expected ok=false when verifying against a different bot token")
	}
}
