package miniapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"lopiibot.com/internal/controller/miniapp/templates"
)

func shellRequest(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group(templates.AppPrefix, authInitData("token", nil))
	group.GET("/overview", func(c *gin.Context) { c.String(http.StatusOK, "leaf") })

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestShellPreservesQueryString(t *testing.T) {
	rec := shellRequest(t, templates.RouteOverview+"?p=month&m=2026-07")

	body := rec.Body.String()
	if !strings.Contains(body, templates.RouteOverview+"?p=month&amp;m=2026-07") &&
		!strings.Contains(body, templates.RouteOverview+"?p=month&m=2026-07") {
		t.Fatalf("shell hx-get lost the query string:\n%s", body)
	}
}
