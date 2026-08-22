package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"lopiibot.com/internal/user"
)

type fakeUsers struct{ u *user.User }

func (f *fakeUsers) FindByChannel(channel, channelUserID string) (*user.User, error) { return f.u, nil }

type fakeReset struct{ called uint64 }

func (f *fakeReset) SoftDeleteByUserID(id uint64) error { f.called = id; return nil }

type fakeEngine struct{ cleared uint64 }

func (f *fakeEngine) Clear(id uint64) error { f.cleared = id; return nil }

func TestReset_SoftDeletesAndClearsFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := &fakeUsers{u: &user.User{ID: 42}}
	accs, movs, eng := &fakeReset{}, &fakeReset{}, &fakeEngine{}
	c := NewController(users, accs, movs, eng, nil) // nil bot: SendMessage guarded

	r := gin.New()
	c.RegisterRoutes(r)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/admin/users/12345/reset", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	if accs.called != 42 || movs.called != 42 {
		t.Errorf("soft-delete not called for user 42: accs=%d movs=%d", accs.called, movs.called)
	}
	if eng.cleared != 42 {
		t.Errorf("engine not cleared for user 42: cleared=%d", eng.cleared)
	}
}
