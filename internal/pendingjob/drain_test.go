package pendingjob

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/trace"
	"lopiibot.com/internal/user"
)

type drainJobs struct {
	jobs     []PendingJob
	deleted  []uint64
	inserted []PendingJob
}

func (d *drainJobs) Insert(j *PendingJob) error {
	d.inserted = append(d.inserted, *j)
	return nil
}
func (d *drainJobs) ListByUserOrdered(uint64) ([]PendingJob, error) { return d.jobs, nil }
func (d *drainJobs) ListPendingUserIDs() ([]uint64, error)          { return []uint64{7}, nil }
func (d *drainJobs) Delete(id uint64) error                         { d.deleted = append(d.deleted, id); return nil }
func (d *drainJobs) CountByUser(uint64) (int64, error)              { return int64(len(d.jobs)), nil }

type fakeUsers struct{}

func (fakeUsers) FindByID(uint64) (*user.User, error)          { return &user.User{}, nil }
func (fakeUsers) FindChannelID(uint64, string) (string, error) { return "100", nil }

type testServices struct {
	users          fakeUsers
	texts          []string
	seenUpdateType string
	seenUserID     *uint64
	tracedFn       func(ctx context.Context, kind string, traceID string, fn func(context.Context) (*uint64, error))
}

func (s *testServices) UsersFindByID(userID uint64) (*user.User, error) {
	return s.users.FindByID(userID)
}
func (s *testServices) UsersFindChannelID(userID uint64, channel string) (string, error) {
	return s.users.FindChannelID(userID, channel)
}
func (s *testServices) HandleFreeText(_ context.Context, _ *bot.Bot, _ int64, _ uint64, text string) error {
	s.texts = append(s.texts, text)
	return nil
}
func (s *testServices) ProceedToUpdateConfirm(_ context.Context, _ *bot.Bot, _ int64, _ uint64, _ string, _ string, _ []string, _ []movement.MovementRow) error {
	return nil
}
func (s *testServices) SendText(_ context.Context, _ *bot.Bot, _ int64, _ string) {}
func (s *testServices) Traced(ctx context.Context, kind string, traceID string, fn func(context.Context) (*uint64, error)) {
	if s.tracedFn != nil {
		s.tracedFn(ctx, kind, traceID, fn)
		return
	}
	traceID2 := trace.NewID()
	ctx = trace.WithID(ctx, traceID2)
	fn(ctx)
}

func TestDrain_GiveUp_OldJob(t *testing.T) {
	payload, _ := json.Marshal(FreeTextPayload{Text: "gasté 5000"})
	jobs := &drainJobs{jobs: []PendingJob{
		{ID: 42, UserID: 7, Kind: KindFreeText, Payload: payload, CreatedAt: time.Now().Add(-3 * time.Hour)},
	}}
	svc := &testServices{}

	drainTick(context.Background(), svc, jobs, nil, time.Now())

	if len(jobs.deleted) != 1 || jobs.deleted[0] != 42 {
		t.Fatalf("want job 42 deleted, got %v", jobs.deleted)
	}
}

func TestDrain_Success_Deletes(t *testing.T) {
	payload, _ := json.Marshal(FreeTextPayload{Text: "hola"})
	jobs := &drainJobs{jobs: []PendingJob{
		{ID: 9, UserID: 7, Kind: KindFreeText, Payload: payload, CreatedAt: time.Now()},
	}}
	svc := &testServices{}

	drainTick(context.Background(), svc, jobs, nil, time.Now())

	if len(jobs.deleted) != 1 || jobs.deleted[0] != 9 {
		t.Fatalf("want job 9 deleted, got %v", jobs.deleted)
	}
}

func TestDrain_ReplayCarriesItsOwnTrace(t *testing.T) {
	payload, _ := json.Marshal(FreeTextPayload{Text: "la panaderia eran 2 mil"})
	jobs := &drainJobs{jobs: []PendingJob{
		{ID: 9, UserID: 7, Kind: KindFreeText, Payload: payload, CreatedAt: time.Now()},
	}}
	var seenTraceID string
	svc := &testServices{}
	svc.tracedFn = func(ctx context.Context, kind string, traceID string, fn func(context.Context) (*uint64, error)) {
		svc.seenUpdateType = kind
		id := trace.NewID()
		ctx = trace.WithID(ctx, id)
		uid, err := fn(ctx)
		svc.seenUserID = uid
		seenTraceID = trace.ID(ctx)
		_ = err
	}

	drainTick(context.Background(), svc, jobs, nil, time.Now())

	if seenTraceID == "" {
		t.Error("el replay corrió sin trace_id: llm_calls e intent_events quedan huérfanos")
	}
	if svc.seenUpdateType != updateTypeReplay {
		t.Errorf("update type want %q, got %q", updateTypeReplay, svc.seenUpdateType)
	}
	if svc.seenUserID == nil || *svc.seenUserID != 7 {
		t.Error("userID want 7")
	}
}
