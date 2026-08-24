//go:build integration

package pendingaction

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"lopiibot.com/internal/database"
	"lopiibot.com/internal/user"
)

// Run with: go test -tags integration ./internal/pendingaction/
// Requires local Postgres (docker compose up -d) with migrations applied.
func TestRepository_DrainsByPositionAndIsPerUser(t *testing.T) {
	conn := testConnection(t)
	r := NewRepository(conn)
	userRepo := user.NewRepository(conn)

	mine := insertUser(t, userRepo)
	theirs := insertUser(t, userRepo)
	t.Cleanup(func() {
		conn.DB.Unscoped().Where("user_id IN ?", []uint64{mine, theirs}).Delete(&PendingAction{})
		conn.DB.Unscoped().Where("id IN ?", []uint64{mine, theirs}).Delete(&user.User{})
	})

	if _, err := r.NextForUser(mine); !errors.Is(err, ErrNoPendingAction) {
		t.Fatalf("want ErrNoPendingAction on an empty queue, got %v", err)
	}

	// Se insertan a propósito fuera de orden: lo que manda es position, no
	// el orden de llegada.
	second := parked(mine, "correct_movement", 1)
	first := parked(mine, "record_movements", 0)
	for _, a := range []*PendingAction{second, first} {
		if err := r.Insert(a); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	other := parked(theirs, "delete_movements", 0)
	if err := r.Insert(other); err != nil {
		t.Fatalf("insert other user: %v", err)
	}

	if n, err := r.CountForUser(mine); err != nil || n != 2 {
		t.Fatalf("want 2 actions for the user, got count=%d err=%v", n, err)
	}

	next, err := r.NextForUser(mine)
	if err != nil {
		t.Fatalf("NextForUser: %v", err)
	}
	if next.ID != first.ID {
		t.Fatalf("want the lowest position (%d, %s), got %d", first.ID, first.Tool, next.ID)
	}
	// Lo guardado tiene que volver entero: el drenaje lo lee de acá.
	var qs []OpenQuestion
	if err := json.Unmarshal(next.Questions, &qs); err != nil {
		t.Fatalf("questions no vuelven como JSON: %v", err)
	}
	if len(qs) != 1 || qs[0].Key != "row0.account" {
		t.Fatalf("las preguntas no sobrevivieron el round-trip: %+v", qs)
	}
	if next.Budget != 3 {
		t.Fatalf("want budget 3, got %d", next.Budget)
	}

	if err := r.Delete(next.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n, err := r.CountForUser(mine); err != nil || n != 1 {
		t.Fatalf("delete tiene que sacar sólo esa fila, got count=%d err=%v", n, err)
	}
	// Y la del otro usuario ni se entera.
	if n, err := r.CountForUser(theirs); err != nil || n != 1 {
		t.Fatalf("la cola del otro usuario cambió: count=%d err=%v", n, err)
	}

	after, err := r.NextForUser(mine)
	if err != nil {
		t.Fatalf("NextForUser tras borrar: %v", err)
	}
	if after.ID != second.ID {
		t.Fatalf("want the next position (%d), got %d", second.ID, after.ID)
	}
}

func parked(userID uint64, tool string, position int) *PendingAction {
	questions, _ := json.Marshal([]OpenQuestion{{
		Key:     "row0.account",
		Prompt:  "¿De qué cuenta salió?",
		Options: []string{"Mercado Pago", "Efectivo"},
	}})
	return &PendingAction{
		UserID:    userID,
		Tool:      tool,
		Payload:   []byte(`{"amount":"1000"}`),
		Questions: questions,
		Budget:    3,
		Position:  position,
		TraceID:   "trace-" + tool,
	}
}

func insertUser(t *testing.T, repo interface{ Insert(*user.User) error }) uint64 {
	t.Helper()
	u := &user.User{Username: fmt.Sprintf("%d", time.Now().UnixNano())}
	if err := repo.Insert(u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return u.ID
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
