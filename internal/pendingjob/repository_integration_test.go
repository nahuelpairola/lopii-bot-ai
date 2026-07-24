//go:build integration

package pendingjob

import (
	"fmt"
	"testing"
	"time"

	"lopiibot.com/internal/database"
	"lopiibot.com/internal/user"
)

// Run with: go test -tags integration ./internal/pendingjob/
// Requires local Postgres (docker compose up -d) with migrations applied.
func TestRepository_InsertListDeleteCount(t *testing.T) {
	conn := testConnection(t)
	r := NewRepository(conn)
	userRepo := user.NewRepository(conn)

	u := &user.User{TelegramID: fmt.Sprintf("%d", time.Now().UnixNano())}
	if err := userRepo.Insert(u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid := u.ID
	t.Cleanup(func() {
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&PendingJob{})
		conn.DB.Unscoped().Where("id = ?", uid).Delete(&user.User{})
	})

	if n, err := r.CountByUser(uid); err != nil || n != 0 {
		t.Fatalf("want 0 jobs before insert, got count=%d err=%v", n, err)
	}

	// Sleep between inserts so created_at is strictly increasing — otherwise
	// millisecond-resolution collisions make the FIFO order assertion flaky.
	var ids []uint64
	for _, text := range []string{"first", "second", "third"} {
		job := &PendingJob{UserID: uid, Kind: "free_text", Payload: []byte(`{"text":"` + text + `"}`)}
		if err := r.Insert(job); err != nil {
			t.Fatalf("insert %q: %v", text, err)
		}
		ids = append(ids, job.ID)
		time.Sleep(5 * time.Millisecond)
	}

	if n, err := r.CountByUser(uid); err != nil || n != 3 {
		t.Fatalf("want 3 jobs, got count=%d err=%v", n, err)
	}

	userIDs, err := r.ListPendingUserIDs()
	if err != nil {
		t.Fatalf("ListPendingUserIDs: %v", err)
	}
	found := false
	for _, id := range userIDs {
		if id == uid {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want %d in ListPendingUserIDs, got %v", uid, userIDs)
	}

	jobs, err := r.ListByUserOrdered(uid)
	if err != nil {
		t.Fatalf("ListByUserOrdered: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("want 3 jobs in order, got %d", len(jobs))
	}
	for i, want := range ids {
		if jobs[i].ID != want {
			t.Fatalf("FIFO order broken: position %d has job %d, want %d", i, jobs[i].ID, want)
		}
	}

	if err := r.Delete(ids[0]); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n, err := r.CountByUser(uid); err != nil || n != 2 {
		t.Fatalf("want 2 jobs after delete, got count=%d err=%v", n, err)
	}
}

// testConnection connects to the local Docker Postgres instance used by integration tests.
func testConnection(t *testing.T) *database.Connection {
	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Port: 5432, Name: "lopiibot", User: "lopiibot", Password: "lopiibot",
	}, false)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	return conn
}
