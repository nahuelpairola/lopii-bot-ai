package miniapp

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/invitation"
	"lopiibot.com/internal/user"
)

// stubAdminUsers devuelve un usuario admin — stubUsers (overview_test.go)
// devuelve uno común, que es justo el caso del 403.
type stubAdminUsers struct{}

func (stubAdminUsers) FindByChannel(channel, channelUserID string) (*user.User, error) {
	return &user.User{ID: 1, IsAdmin: true}, nil
}

type stubInvitations struct {
	list    []invitation.Invitation
	created int
}

func (s *stubInvitations) Create(createdBy uint64) (*invitation.Invitation, error) {
	s.created++
	inv := invitation.Invitation{Code: "NEW999", CreatedBy: createdBy, ExpiresAt: time.Now().Add(time.Hour)}
	s.list = append([]invitation.Invitation{inv}, s.list...)
	return &inv, nil
}

func (s *stubInvitations) List() ([]invitation.Invitation, error) { return s.list, nil }

func TestHandleAdmin_ForbiddenForNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := NewController(stubMovements{}, stubAccounts{}, stubIcons{}, stubUsers{}, &stubInvitations{}, testBotToken, testBotUsername)

	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/admin"))

	if w.Code != 403 {
		t.Fatalf("un usuario común tiene que comerse un 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleAdmin_RendersTheThreeStates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	used := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	invs := []invitation.Invitation{
		{Code: "PEND01", ExpiresAt: time.Now().Add(time.Hour)},
		{Code: "USED01", ExpiresAt: time.Now().Add(time.Hour), UsedAt: &used},
		{Code: "OLD001", ExpiresAt: time.Now().Add(-time.Hour)},
	}
	c := NewController(stubMovements{}, stubAccounts{}, stubIcons{}, stubAdminUsers{}, &stubInvitations{list: invs}, testBotToken, testBotUsername)

	router := gin.New()
	c.RegisterRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authedHTMXRequest(t, "/app/admin"))

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	html := w.Body.String()
	for _, want := range []string{"pendiente", "usada", "expirada", "03/08/2026"} {
		if !strings.Contains(html, want) {
			t.Errorf("falta %q:\n%s", want, html)
		}
	}
	// Sólo la pendiente lleva link, y lleva el username del bot configurado.
	if !strings.Contains(html, "https://t.me/"+testBotUsername+"?start=PEND01") {
		t.Errorf("la pendiente tendría que traer su link:\n%s", html)
	}
	if strings.Contains(html, "start=OLD001") {
		t.Errorf("una vencida no se comparte:\n%s", html)
	}
}

func TestHandleCreateInvitation_CreatesAndRerenders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	invitations := &stubInvitations{}
	c := NewController(stubMovements{}, stubAccounts{}, stubIcons{}, stubAdminUsers{}, invitations, testBotToken, testBotUsername)

	router := gin.New()
	c.RegisterRoutes(router)

	req := authedHTMXRequest(t, "/app/admin/invitations")
	req.Method = "POST"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if invitations.created != 1 {
		t.Errorf("invitaciones creadas = %d, want 1", invitations.created)
	}
	// La respuesta del POST es la vista entera, con la nueva ya adentro.
	if !strings.Contains(w.Body.String(), "NEW999") {
		t.Errorf("el POST tendría que devolver la vista con la nueva invitación:\n%s", w.Body.String())
	}
}
