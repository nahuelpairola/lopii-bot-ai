package miniapp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func timeNow() time.Time { return time.Now() }

// authedHTMXRequest builds the request an in-Telegram htmx call makes: the
// HX-Request marker plus a validly-signed initData header (what app.js
// injects client-side). This is the ONLY path that renders data partials.
func authedHTMXRequest(t *testing.T, path string) *http.Request {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("X-Telegram-Init-Data", buildInitData(t, "999", timeNow(), testBotToken))
	return req
}
