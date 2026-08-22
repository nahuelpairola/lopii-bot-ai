//go:build integration

package nudges

import (
	"fmt"
	"testing"
	"time"

	"lopiibot.com/internal/database"
	"lopiibot.com/internal/user"
)

// Run with: go test -tags integration ./internal/nudge/
// Requires local Postgres (docker compose up -d) with migrations applied.
//
// Estas dos garantías viven en el SQL, no en Go: un mock del repositorio
// probaría el mock. Por eso son un test de integración y no unitario.
func TestRepository_MarkSentIsOnceEverButMarkSentAgainAdvances(t *testing.T) {
	conn := testConnection(t)
	r := NewRepository(conn)
	uid := newTestUser(t, conn)

	const key = "integration_recurring_tip"

	if err := r.MarkSent(uid, key); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	first := sentAt(t, conn, uid, key)

	time.Sleep(10 * time.Millisecond)

	// MarkSent es once-ever: un segundo envío NO toca la fila. Es lo que hace
	// que un tip no se repita nunca.
	if err := r.MarkSent(uid, key); err != nil {
		t.Fatalf("MarkSent (segunda): %v", err)
	}
	if got := sentAt(t, conn, uid, key); !got.Equal(first) {
		t.Errorf("MarkSent movió sent_at de %v a %v; debería ser once-ever", first, got)
	}

	// MarkSentAgain sí lo actualiza. Sin esto LastSentAt queda congelada en el
	// primer envío, el cooldown se lee como cumplido para siempre y el tip
	// recurrente sale en CADA mensaje.
	if err := r.MarkSentAgain(uid, key); err != nil {
		t.Fatalf("MarkSentAgain: %v", err)
	}
	second := sentAt(t, conn, uid, key)
	if !second.After(first) {
		t.Errorf("MarkSentAgain dejó sent_at en %v (era %v); tiene que avanzar", second, first)
	}

	// Y LastSentAt, que es lo que lee el cooldown, tiene que ver ese avance.
	last, err := r.LastSentAt(uid)
	if err != nil || last == nil {
		t.Fatalf("LastSentAt: %v (nil=%v)", err, last == nil)
	}
	if !last.Equal(second) {
		t.Errorf("LastSentAt = %v, want %v", *last, second)
	}
}

func TestRepository_MarkTappedIsSetOnce(t *testing.T) {
	conn := testConnection(t)
	r := NewRepository(conn)
	uid := newTestUser(t, conn)

	const key = "integration_tapped_tip"

	if err := r.MarkTapped(uid, key); err != nil {
		t.Fatalf("MarkTapped sin fila: %v", err)
	}
	if n := countRows(t, conn, uid); n != 0 {
		t.Fatalf("un tap sin tip enviado no debería crear filas, hay %d", n)
	}

	if err := r.MarkSent(uid, key); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	if err := r.MarkTapped(uid, key); err != nil {
		t.Fatalf("MarkTapped: %v", err)
	}
	first := tappedAt(t, conn, uid, key)
	if first == nil {
		t.Fatal("el primer tap tiene que sellar tapped_at")
	}

	time.Sleep(10 * time.Millisecond)

	// El segundo tap NO pisa el timestamp: así "% tap" mide usuarios que
	// engancharon, no cantidad de taps.
	if err := r.MarkTapped(uid, key); err != nil {
		t.Fatalf("MarkTapped (segunda): %v", err)
	}
	second := tappedAt(t, conn, uid, key)
	if second == nil || !second.Equal(*first) {
		t.Errorf("el segundo tap movió tapped_at de %v a %v; debería ser set-once", first, second)
	}
}

func TestRepository_SentKeys(t *testing.T) {
	conn := testConnection(t)
	r := NewRepository(conn)
	uid := newTestUser(t, conn)

	keys, err := r.SentKeys(uid)
	if err != nil || len(keys) != 0 {
		t.Fatalf("usuario nuevo: want 0 keys, got %v err=%v", keys, err)
	}

	for _, k := range []string{"a_tip", "b_tip"} {
		if err := r.MarkSent(uid, k); err != nil {
			t.Fatalf("MarkSent %q: %v", k, err)
		}
	}
	keys, err = r.SentKeys(uid)
	if err != nil {
		t.Fatalf("SentKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("want 2 keys, got %v", keys)
	}
}

func newTestUser(t *testing.T, conn *database.Connection) uint64 {
	t.Helper()
	userRepo := user.NewRepository(conn)
	u := &user.User{Username: fmt.Sprintf("%d", time.Now().UnixNano())}
	if err := userRepo.Insert(u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid := u.ID
	t.Cleanup(func() {
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&UserNudge{})
		conn.DB.Unscoped().Where("id = ?", uid).Delete(&user.User{})
	})
	return uid
}

func sentAt(t *testing.T, conn *database.Connection, uid uint64, key string) time.Time {
	t.Helper()
	var n UserNudge
	if err := conn.DB.Where("user_id = ? AND nudge_key = ?", uid, key).First(&n).Error; err != nil {
		t.Fatalf("leer la fila: %v", err)
	}
	return n.SentAt
}

func tappedAt(t *testing.T, conn *database.Connection, uid uint64, key string) *time.Time {
	t.Helper()
	var n UserNudge
	if err := conn.DB.Where("user_id = ? AND nudge_key = ?", uid, key).First(&n).Error; err != nil {
		t.Fatalf("leer la fila: %v", err)
	}
	return n.TappedAt
}

func countRows(t *testing.T, conn *database.Connection, uid uint64) int64 {
	t.Helper()
	var n int64
	if err := conn.DB.Model(&UserNudge{}).Where("user_id = ?", uid).Count(&n).Error; err != nil {
		t.Fatalf("contar filas: %v", err)
	}
	return n
}

func testConnection(t *testing.T) *database.Connection {
	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Port: 5432, Name: "lopiibot", User: "lopiibot", Password: "lopiibot",
	}, false)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	return conn
}
