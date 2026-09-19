package pendingjob

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/trace"
	"lopiibot.com/internal/user"
)

type drainJobs struct {
	jobs       []PendingJob
	deleted    []uint64
	inserted   []PendingJob
	users      []uint64
	deleteErr  error
	notClaimed bool
	insertErr  error
}

func (d *drainJobs) Insert(j *PendingJob) error {
	if d.insertErr != nil {
		return d.insertErr
	}
	d.inserted = append(d.inserted, *j)
	return nil
}
func (d *drainJobs) ListByUserOrdered(userID uint64) ([]PendingJob, error) {
	var out []PendingJob
	for _, j := range d.jobs {
		if j.UserID == userID {
			out = append(out, j)
		}
	}
	return out, nil
}
func (d *drainJobs) ListPendingUserIDs() ([]uint64, error) {
	if len(d.users) > 0 {
		return d.users, nil
	}
	return []uint64{7}, nil
}
func (d *drainJobs) Delete(id uint64) (bool, error) {
	if d.deleteErr != nil {
		return false, d.deleteErr
	}
	if d.notClaimed {
		return false, nil
	}
	d.deleted = append(d.deleted, id)
	return true, nil
}
func (d *drainJobs) CountByUser(uint64) (int64, error) { return int64(len(d.jobs)), nil }

type fakeUsers struct{}

func (fakeUsers) FindByID(uint64) (*user.User, error) { return &user.User{}, nil }

type fakeChats struct{ chat *messenger.FakeChat }

func (f fakeChats) ChatFor(uint64) (messenger.Chat, error) { return f.chat, nil }

type testServices struct {
	users          fakeUsers
	usersErr       error
	texts          []string
	onFreeText     func(ctx context.Context, text string) error
	seenUpdateType string
	seenUserID     *uint64
	tracedFn       func(ctx context.Context, kind string, traceID string, fn func(context.Context) (*uint64, error))
}

func (s *testServices) UsersFindByID(userID uint64) (*user.User, error) {
	if s.usersErr != nil {
		return nil, s.usersErr
	}
	return s.users.FindByID(userID)
}
func (s *testServices) HandleFreeText(ctx context.Context, _ messenger.Chat, _ uint64, text string) error {
	s.texts = append(s.texts, text)
	if s.onFreeText != nil {
		return s.onFreeText(ctx, text)
	}
	return nil
}
func (s *testServices) ProceedToUpdateConfirm(_ context.Context, _ messenger.Chat, _ uint64, _ string, _ string, _ []string, _ []movement.MovementRow) error {
	return nil
}
func (s *testServices) SendText(ctx context.Context, chat messenger.Chat, text string) {
	_ = messenger.SendText(ctx, chat, text)
}
func (s *testServices) Traced(ctx context.Context, kind string, traceID string, fn func(context.Context) (*uint64, error)) {
	if s.tracedFn != nil {
		s.tracedFn(ctx, kind, traceID, fn)
		return
	}
	traceID2 := trace.NewID()
	ctx = trace.WithID(ctx, traceID2)
	fn(ctx)
}

func freeTextPayload(t *testing.T, text string) []byte {
	t.Helper()
	p, err := json.Marshal(FreeTextPayload{Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func resetDrainGate(t *testing.T) {
	t.Helper()
	reset := func() {
		drainMu.Lock()
		nextDrainAt = time.Time{}
		drainMu.Unlock()
	}
	reset()
	t.Cleanup(reset)
}

func sentTexts(c *messenger.FakeChat) []string {
	out := make([]string, 0, len(c.Sent))
	for _, p := range c.Sent {
		out = append(out, p.Text)
	}
	return out
}

func TestDrain_GiveUp_OldJob(t *testing.T) {
	payload, _ := json.Marshal(FreeTextPayload{Text: "gasté 5000"})
	jobs := &drainJobs{jobs: []PendingJob{
		{ID: 42, UserID: 7, Kind: KindFreeText, Payload: payload, CreatedAt: time.Now().Add(-3 * time.Hour)},
	}}
	svc := &testServices{}
	chats := fakeChats{chat: &messenger.FakeChat{}}

	drainTick(context.Background(), svc, jobs, chats, time.Now())

	if len(jobs.deleted) != 1 || jobs.deleted[0] != 42 {
		t.Fatalf("want job 42 deleted, got %v", jobs.deleted)
	}
	if len(chats.chat.Sent) != 1 {
		t.Fatalf("want the give-up message sent, got %v", chats.chat.Sent)
	}
}

func TestDrain_NonWritingReplay_DeletesJobAfterwards(t *testing.T) {
	payload, _ := json.Marshal(FreeTextPayload{Text: "hola"})
	jobs := &drainJobs{jobs: []PendingJob{
		{ID: 9, UserID: 7, Kind: KindFreeText, Payload: payload, CreatedAt: time.Now()},
	}}
	svc := &testServices{}
	chats := fakeChats{chat: &messenger.FakeChat{}}

	drainTick(context.Background(), svc, jobs, chats, time.Now())

	if len(jobs.deleted) != 1 || jobs.deleted[0] != 9 {
		t.Fatalf("want job 9 deleted, got %v", jobs.deleted)
	}
	if len(svc.texts) != 1 || svc.texts[0] != "hola" {
		t.Fatalf("want the replayed text handled, got %v", svc.texts)
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
	chats := fakeChats{chat: &messenger.FakeChat{}}

	drainTick(context.Background(), svc, jobs, chats, time.Now())

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

func TestDrain_ReplayThatWrites_ClaimsBeforeWriting(t *testing.T) {
	resetDrainGate(t)
	jobs := &drainJobs{jobs: []PendingJob{{ID: 9, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "gasté 5000 en el super"), CreatedAt: time.Now()}}}
	var deletedAtWrite []uint64
	svc := &testServices{onFreeText: func(ctx context.Context, _ string) error {
		if err := ClaimReplay(ctx); err != nil {
			return err
		}
		deletedAtWrite = append([]uint64(nil), jobs.deleted...)
		return nil
	}}

	drainTick(context.Background(), svc, jobs, fakeChats{chat: &messenger.FakeChat{}}, time.Now())

	if len(deletedAtWrite) != 1 || deletedAtWrite[0] != 9 {
		t.Fatalf("the job must be gone before the write, deleted at write time: %v", deletedAtWrite)
	}
	if len(jobs.deleted) != 1 {
		t.Fatalf("job deleted %d times, want once: the claim is the only delete", len(jobs.deleted))
	}
}

func TestDrain_ClaimedElsewhere_WritesNothingAndSendsNothing(t *testing.T) {
	resetDrainGate(t)
	jobs := &drainJobs{notClaimed: true, jobs: []PendingJob{{ID: 9, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "gasté 5000 en el super"), CreatedAt: time.Now()}}}
	wrote := false
	svc := &testServices{onFreeText: func(ctx context.Context, _ string) error {
		if err := ClaimReplay(ctx); err != nil {
			return err
		}
		wrote = true
		return nil
	}}
	chat := &messenger.FakeChat{}

	drainTick(context.Background(), svc, jobs, fakeChats{chat: chat}, time.Now())

	if wrote {
		t.Fatal("wrote after losing the claim: two instances would record the money twice")
	}
	if len(chat.Sent) != 0 {
		t.Fatalf("the losing instance must stay silent, sent %v", sentTexts(chat))
	}
}

func TestDrain_ClaimFails_JobStaysForNextTick(t *testing.T) {
	resetDrainGate(t)
	jobs := &drainJobs{deleteErr: errors.New("connection reset"), jobs: []PendingJob{
		{ID: 9, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "gasté 500"), CreatedAt: time.Now()},
		{ID: 10, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "no, 600"), CreatedAt: time.Now()},
	}}
	svc := &testServices{onFreeText: func(ctx context.Context, _ string) error { return ClaimReplay(ctx) }}
	chat := &messenger.FakeChat{}

	drainTick(context.Background(), svc, jobs, fakeChats{chat: chat}, time.Now())

	if len(svc.texts) != 1 {
		t.Fatalf("a failed claim must stop this user's drain so \"no, 600\" never overtakes \"gasté 500\", replayed %v", svc.texts)
	}
	if len(chat.Sent) != 0 {
		t.Fatalf("the next tick retries, so the user is not told anything yet, sent %v", sentTexts(chat))
	}
}

func TestDrain_RateLimitedBeforeClaim_LeavesJobUntouched(t *testing.T) {
	resetDrainGate(t)
	jobs := &drainJobs{jobs: []PendingJob{{ID: 9, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "gasté 5000"), CreatedAt: time.Now()}}}
	svc := &testServices{onFreeText: func(context.Context, string) error {
		return &orchestrator.RateLimitedError{RetryAfter: time.Millisecond}
	}}

	drainTick(context.Background(), svc, jobs, fakeChats{chat: &messenger.FakeChat{}}, time.Now())

	if len(jobs.deleted) != 0 || len(jobs.inserted) != 0 {
		t.Fatalf("a 429 before the claim must leave the row alone, deleted=%v inserted=%v", jobs.deleted, jobs.inserted)
	}
}

func TestDrain_RateLimitedAfterClaim_ReinsertsWithOriginalCreatedAt(t *testing.T) {
	resetDrainGate(t)
	created := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	payload := freeTextPayload(t, "gasté 5000")
	jobs := &drainJobs{jobs: []PendingJob{{ID: 9, UserID: 7, Kind: KindFreeText, Payload: payload, CreatedAt: created}}}
	svc := &testServices{onFreeText: func(ctx context.Context, _ string) error {
		if err := ClaimReplay(ctx); err != nil {
			return err
		}
		return &orchestrator.RateLimitedError{RetryAfter: time.Millisecond}
	}}

	drainTick(context.Background(), svc, jobs, fakeChats{chat: &messenger.FakeChat{}}, time.Now())

	if len(jobs.inserted) != 1 {
		t.Fatalf("a claimed job that hit a 429 must go back to the queue, inserted %v", jobs.inserted)
	}
	back := jobs.inserted[0]
	if back.ID != 0 || back.Kind != KindFreeText || string(back.Payload) != string(payload) || !back.CreatedAt.Equal(created) {
		t.Fatalf("re-inserted %+v: it must keep Kind, Payload and the original CreatedAt, or MaxJobAge never fires", back)
	}
}

func TestDrain_ReinsertFails_UserIsTold(t *testing.T) {
	resetDrainGate(t)
	jobs := &drainJobs{insertErr: errors.New("connection reset"), jobs: []PendingJob{{ID: 9, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "gasté 5000"), CreatedAt: time.Now()}}}
	svc := &testServices{onFreeText: func(ctx context.Context, _ string) error {
		if err := ClaimReplay(ctx); err != nil {
			return err
		}
		return &orchestrator.RateLimitedError{RetryAfter: time.Millisecond}
	}}
	chat := &messenger.FakeChat{}

	drainTick(context.Background(), svc, jobs, fakeChats{chat: chat}, time.Now())

	sent := sentTexts(chat)
	if len(sent) != 1 || !strings.Contains(sent[0], "gasté 5000") {
		t.Fatalf("the job is gone and could not be re-queued: the user must be told, sent %v", sent)
	}
}

func TestDrain_UserLookupFails_OldJobStillGivesUp(t *testing.T) {
	resetDrainGate(t)
	jobs := &drainJobs{jobs: []PendingJob{{ID: 42, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "gasté 5000"), CreatedAt: time.Now().Add(-3 * time.Hour)}}}
	svc := &testServices{usersErr: errors.New("user not found")}

	drainTick(context.Background(), svc, jobs, fakeChats{chat: &messenger.FakeChat{}}, time.Now())

	if len(jobs.deleted) != 1 || jobs.deleted[0] != 42 {
		t.Fatalf("a job past MaxJobAge must be dropped even when the lookup fails, deleted %v", jobs.deleted)
	}
}

func TestDrain_GiveUp_AlreadyGoneSendsNothing(t *testing.T) {
	resetDrainGate(t)
	jobs := &drainJobs{notClaimed: true, jobs: []PendingJob{{ID: 42, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "gasté 5000"), CreatedAt: time.Now().Add(-3 * time.Hour)}}}
	chat := &messenger.FakeChat{}

	drainTick(context.Background(), &testServices{}, jobs, fakeChats{chat: chat}, time.Now())

	if len(chat.Sent) != 0 {
		t.Fatalf("another instance already gave up on this job and told the user, sent again %v", sentTexts(chat))
	}
}

func TestReplayJob_CorruptPayload_ReturnsError(t *testing.T) {
	err := replayJob(context.Background(), &testServices{}, &messenger.FakeChat{}, 7, PendingJob{Kind: KindFreeText, Payload: []byte("{")})
	if err == nil {
		t.Fatal("a corrupt payload was reported as a successful replay")
	}
}

func TestDrain_ShutdownSignal_StopsStartingNewJobs(t *testing.T) {
	resetDrainGate(t)
	jobs := &drainJobs{jobs: []PendingJob{{ID: 9, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "gasté 5000"), CreatedAt: time.Now()}}}
	svc := &testServices{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	drainTick(ctx, svc, jobs, fakeChats{chat: &messenger.FakeChat{}}, time.Now())

	if len(svc.texts) != 0 || len(jobs.deleted) != 0 {
		t.Fatalf("after the shutdown signal no job may start, replayed %v deleted %v", svc.texts, jobs.deleted)
	}
}

func TestDrain_ReplayContextSurvivesShutdown(t *testing.T) {
	resetDrainGate(t)
	jobs := &drainJobs{jobs: []PendingJob{{ID: 9, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "gasté 5000"), CreatedAt: time.Now()}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := &testServices{onFreeText: func(rctx context.Context, _ string) error {
		cancel()
		if rctx.Err() != nil {
			t.Error("SIGTERM cancelled a replay in flight: its Groq calls die and the message is lost")
		}
		return nil
	}}

	drainTick(ctx, svc, jobs, fakeChats{chat: &messenger.FakeChat{}}, time.Now())
}

func TestDrain_ReplayPanics_JobStaysAndOtherUsersDrain(t *testing.T) {
	resetDrainGate(t)
	jobs := &drainJobs{users: []uint64{7, 8}, jobs: []PendingJob{
		{ID: 9, UserID: 7, Kind: KindFreeText, Payload: freeTextPayload(t, "boom"), CreatedAt: time.Now()},
		{ID: 10, UserID: 8, Kind: KindFreeText, Payload: freeTextPayload(t, "hola"), CreatedAt: time.Now()},
	}}
	svc := &testServices{onFreeText: func(_ context.Context, text string) error {
		if text == "boom" {
			panic("boom")
		}
		return nil
	}}

	drainTick(context.Background(), svc, jobs, fakeChats{chat: &messenger.FakeChat{}}, time.Now())

	if len(jobs.deleted) != 1 || jobs.deleted[0] != 10 {
		t.Fatalf("the panicking job must stay for a retry and the other user must drain, deleted %v", jobs.deleted)
	}
}
