//go:build integration

package metric

import (
	"fmt"
	"testing"
	"time"

	"lopiibot.com/internal/database"
	"lopiibot.com/internal/user"
)

func TestDeleteOlderThan_PurgesLLMCallsRequestTracesAndIntentEvents(t *testing.T) {
	conn := testConnection(t)
	r := InitRepository(conn)
	userRepo := user.NewRepository(conn)

	u := &user.User{Username: fmt.Sprintf("%d", time.Now().UnixNano())}
	if err := userRepo.Insert(u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	uid := u.ID
	t.Cleanup(func() {
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&IntentEvent{})
		conn.DB.Unscoped().Where("user_id = ?", uid).Delete(&RequestTrace{})
		conn.DB.Unscoped().Where("id = ?", uid).Delete(&user.User{})
	})

	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	old := cutoff.Add(-time.Hour)
	recent := cutoff.Add(time.Hour)

	oldCall := &LLMCall{TraceID: "old", CallType: "test", Model: "test", LatencyMs: 1}
	if err := r.InsertLLMCall(oldCall); err != nil {
		t.Fatalf("insert old llm_call: %v", err)
	}
	recentCall := &LLMCall{TraceID: "recent", CallType: "test", Model: "test", LatencyMs: 1}
	if err := r.InsertLLMCall(recentCall); err != nil {
		t.Fatalf("insert recent llm_call: %v", err)
	}

	if err := r.InsertRequestTrace("old-trace", &uid, "text", old, 1, ""); err != nil {
		t.Fatalf("insert old request_trace: %v", err)
	}
	if err := r.InsertRequestTrace("recent-trace", &uid, "text", recent, 1, ""); err != nil {
		t.Fatalf("insert recent request_trace: %v", err)
	}

	if err := r.Log(uid, "old-intent-trace", "old message", "CREATE", false, "confirmed"); err != nil {
		t.Fatalf("insert old intent_event: %v", err)
	}
	if err := r.Log(uid, "recent-intent-trace", "recent message", "CREATE", false, "confirmed"); err != nil {
		t.Fatalf("insert recent intent_event: %v", err)
	}

	if err := conn.DB.Model(&LLMCall{}).Where("id = ?", oldCall.ID).Update("created_at", old).Error; err != nil {
		t.Fatalf("backdate llm_call: %v", err)
	}
	if err := conn.DB.Model(&RequestTrace{}).Where("trace_id = ?", "old-trace").Update("created_at", old).Error; err != nil {
		t.Fatalf("backdate request_trace: %v", err)
	}
	if err := conn.DB.Model(&IntentEvent{}).Where("trace_id = ?", "old-intent-trace").Update("created_at", old).Error; err != nil {
		t.Fatalf("backdate intent_event: %v", err)
	}

	if err := r.DeleteOlderThan(cutoff); err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}

	var n int64
	if err := conn.DB.Model(&LLMCall{}).Where("id = ?", oldCall.ID).Count(&n).Error; err != nil {
		t.Fatalf("count old llm_call: %v", err)
	}
	if n != 0 {
		t.Fatalf("want old llm_call purged, still found %d rows", n)
	}
	if err := conn.DB.Model(&LLMCall{}).Where("id = ?", recentCall.ID).Count(&n).Error; err != nil {
		t.Fatalf("count recent llm_call: %v", err)
	}
	if n != 1 {
		t.Fatalf("want recent llm_call kept, found %d rows", n)
	}

	if err := conn.DB.Model(&RequestTrace{}).Where("trace_id = ?", "old-trace").Count(&n).Error; err != nil {
		t.Fatalf("count old request_trace: %v", err)
	}
	if n != 0 {
		t.Fatalf("want old request_trace purged, still found %d rows", n)
	}
	if err := conn.DB.Model(&RequestTrace{}).Where("trace_id = ?", "recent-trace").Count(&n).Error; err != nil {
		t.Fatalf("count recent request_trace: %v", err)
	}
	if n != 1 {
		t.Fatalf("want recent request_trace kept, found %d rows", n)
	}

	if err := conn.DB.Model(&IntentEvent{}).Where("trace_id = ?", "old-intent-trace").Count(&n).Error; err != nil {
		t.Fatalf("count old intent_event: %v", err)
	}
	if n != 0 {
		t.Fatalf("want old intent_event purged, still found %d rows", n)
	}
	if err := conn.DB.Model(&IntentEvent{}).Where("trace_id = ?", "recent-intent-trace").Count(&n).Error; err != nil {
		t.Fatalf("count recent intent_event: %v", err)
	}
	if n != 1 {
		t.Fatalf("want recent intent_event kept, found %d rows", n)
	}
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
