//go:build integration

package reminder

import (
	"slices"
	"testing"
	"time"

	"lopiibot.com/internal/database"
)

func testRepo(t *testing.T) *repository {
	t.Helper()
	conn, err := database.Initialize(database.Creds{
		Host: "localhost", Name: "lopiibot", Port: 5432,
		User: "lopiibot", Password: "lopiibot",
	}, false)
	if err != nil {
		t.Fatalf("failed to initialize database: %v", err)
	}
	return NewRepository(conn)
}

func allUserIDs(t *testing.T, r *repository) []uint64 {
	t.Helper()
	var ids []uint64
	if err := r.conn.DB.Table("users").Order("id").Pluck("id", &ids).Error; err != nil {
		t.Fatalf("list users: %v", err)
	}
	return ids
}

func TestListMonthlyDue_ReachesEveryUserIncludingThoseWithNoReminderRow(t *testing.T) {
	r := testRepo(t)
	if err := r.conn.DB.Exec("UPDATE reminders SET last_monthly_summary_on = NULL").Error; err != nil {
		t.Fatalf("reset: %v", err)
	}

	due, err := r.ListMonthlyDue(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListMonthlyDue: %v", err)
	}

	want := allUserIDs(t, r)
	if len(want) == 0 {
		t.Skip("no users in the local database")
	}
	if !slices.Equal(due, want) {
		t.Errorf("due = %v, want every user %v — the monthly summary is not opt-in", due, want)
	}
}

func TestListMonthlyDue_IgnoresTheWeeklyOptOut(t *testing.T) {
	r := testRepo(t)
	ids := allUserIDs(t, r)
	if len(ids) == 0 {
		t.Skip("no users in the local database")
	}
	target := ids[0]

	if err := r.Upsert(&Reminder{UserID: target, Enabled: false, WeeklySummaryEnabled: false}); err != nil {
		t.Fatalf("seed opt-out: %v", err)
	}
	if err := r.conn.DB.Exec("UPDATE reminders SET last_monthly_summary_on = NULL WHERE user_id = ?", target).Error; err != nil {
		t.Fatalf("reset: %v", err)
	}

	due, err := r.ListMonthlyDue(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ListMonthlyDue: %v", err)
	}
	if !slices.Contains(due, target) {
		t.Errorf("user %d with weekly_summary_enabled=false is missing: the monthly must not be opt-in", target)
	}
}

func TestSetLastMonthlySummaryOn_CreatesTheRowWithEveryFlagOff(t *testing.T) {
	r := testRepo(t)
	ids := allUserIDs(t, r)
	if len(ids) == 0 {
		t.Skip("no users in the local database")
	}
	target := ids[len(ids)-1]
	if err := r.conn.DB.Exec("DELETE FROM reminders WHERE user_id = ?", target).Error; err != nil {
		t.Fatalf("drain: %v", err)
	}

	day := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	if err := r.SetLastMonthlySummaryOn(target, day); err != nil {
		t.Fatalf("SetLastMonthlySummaryOn: %v", err)
	}

	rem, err := r.FindByUserID(target)
	if err != nil {
		t.Fatalf("FindByUserID: %v", err)
	}
	if rem.LastMonthlySummaryOn == nil || !rem.LastMonthlySummaryOn.Equal(day) {
		t.Errorf("LastMonthlySummaryOn = %v, want %v", rem.LastMonthlySummaryOn, day)
	}
	if rem.Enabled {
		t.Error("the daily reminder was switched on by a monthly send")
	}
	if rem.WeeklySummaryEnabled {
		t.Error("the weekly summary was switched on by a monthly send")
	}

	if err := r.conn.DB.Exec("DELETE FROM reminders WHERE user_id = ?", target).Error; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}
