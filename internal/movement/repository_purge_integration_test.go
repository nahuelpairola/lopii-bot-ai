//go:build integration

package movement

import (
	"slices"
	"testing"
	"time"
)

func TestPurgeSoftDeletedBefore_DeletesOnlyRowsSoftDeletedBeforeTheCutoff(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	now := time.Now()

	live := seedDeletableMovement(t, conn, r)
	recent := seedDeletableMovement(t, conn, r)
	old := seedDeletableMovement(t, conn, r)
	setDeletedAt := func(id uint, at time.Time) {
		t.Helper()
		if err := conn.DB.Exec("UPDATE movements SET deleted_at = ? WHERE id = ?", at, id).Error; err != nil {
			t.Fatalf("set deleted_at: %v", err)
		}
	}
	setDeletedAt(recent, now.AddDate(0, 0, -10))
	setDeletedAt(old, now.AddDate(0, 0, -100))

	purged, err := r.PurgeSoftDeletedBefore(now.AddDate(0, 0, -90))
	if err != nil {
		t.Fatalf("purge: %v", err)
	}

	ids := make([]uint, len(purged))
	for i, p := range purged {
		ids[i] = p.ID
		if p.ID == old && p.UserID != 1 {
			t.Errorf("purged user_id = %d, want 1", p.UserID)
		}
	}
	if !slices.Contains(ids, old) {
		t.Errorf("purged %v, want it to contain the row deleted 100 days ago (%d)", ids, old)
	}
	if slices.Contains(ids, live) || slices.Contains(ids, recent) {
		t.Errorf("purged %v, must not contain live (%d) or recently deleted (%d)", ids, live, recent)
	}

	var remaining []uint
	if err := conn.DB.Raw("SELECT id FROM movements WHERE id IN ?", []uint{live, recent, old}).Scan(&remaining).Error; err != nil {
		t.Fatalf("select remaining: %v", err)
	}
	slices.Sort(remaining)
	if want := []uint{live, recent}; !slices.Equal(remaining, want) {
		t.Errorf("rows left = %v, want %v", remaining, want)
	}
}
