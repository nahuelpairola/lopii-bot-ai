package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
)

type fakeTraceRepo struct {
	traceID  string
	userID   *uint64
	updType  string
	errMsg   string
	inserted bool
}

func (f *fakeTraceRepo) InsertRequestTrace(traceID string, userID *uint64, updateType string, receivedAt time.Time, latencyMs int, errMsg string) error {
	f.traceID, f.userID, f.updType, f.errMsg, f.inserted = traceID, userID, updateType, errMsg, true
	return nil
}

func TestWithTraceRecordsSpine(t *testing.T) {
	tr := &fakeTraceRepo{}
	c := &controller{traces: tr}
	uid := uint64(7)
	upd := &models.Update{CallbackQuery: &models.CallbackQuery{}}

	c.withTrace(context.Background(), upd, func(ctx context.Context) (*uint64, error) {
		return &uid, nil
	})

	if !tr.inserted || tr.traceID == "" || tr.userID == nil || *tr.userID != 7 || tr.updType != "callback" {
		t.Fatalf("spine not recorded correctly: %+v", tr)
	}
}
