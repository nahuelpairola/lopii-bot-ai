package pendingjob

import (
	"context"
	"testing"
	"time"

	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/user"
)

type fakeJobs struct {
	inserted []PendingJob
	count    int64
}

func (f *fakeJobs) Insert(j *PendingJob) error {
	f.inserted = append(f.inserted, *j)
	return nil
}
func (f *fakeJobs) ListByUserOrdered(uint64) ([]PendingJob, error) { return f.inserted, nil }
func (f *fakeJobs) ListPendingUserIDs() ([]uint64, error)          { return nil, nil }
func (f *fakeJobs) Delete(uint64) (bool, error)                    { return true, nil }
func (f *fakeJobs) CountByUser(uint64) (int64, error)              { return f.count, nil }

type enqueueTestServices struct {
	texts []string
}

func (s *enqueueTestServices) UsersFindByID(uint64) (*user.User, error) { return nil, nil }
func (s *enqueueTestServices) HandleFreeText(_ context.Context, _ messenger.Chat, _ uint64, text string) error {
	s.texts = append(s.texts, text)
	return nil
}
func (s *enqueueTestServices) ProceedToUpdateConfirm(_ context.Context, _ messenger.Chat, _ uint64, _ string, _ string, _ []string, _ []movement.MovementRow) error {
	return nil
}
func (s *enqueueTestServices) SendText(ctx context.Context, chat messenger.Chat, text string) {
	s.texts = append(s.texts, text)
	_ = messenger.SendText(ctx, chat, text)
}
func (s *enqueueTestServices) Traced(_ context.Context, _ string, _ string, fn func(context.Context) (*uint64, error)) {
	ctx := context.Background()
	fn(ctx)
}

func TestEnqueueFreeText_RateLimited_Enqueues(t *testing.T) {
	jobs := &fakeJobs{}
	svc := &enqueueTestServices{}
	chat := &messenger.FakeChat{}
	err := &orchestrator.RateLimitedError{RetryAfter: 8 * time.Second}

	ok := EnqueueFreeText(context.Background(), svc, jobs, chat, 7, "gasté 5000 en el super", err)
	if !ok {
		t.Fatal("want enqueued (true)")
	}
	if len(jobs.inserted) != 1 || jobs.inserted[0].Kind != KindFreeText {
		t.Fatalf("want 1 free_text job, got %+v", jobs.inserted)
	}
	if len(chat.Sent) != 1 {
		t.Fatalf("want the ack sent, got %v", chat.Sent)
	}
}

func TestEnqueueFreeText_NotRateLimited_NoEnqueue(t *testing.T) {
	jobs := &fakeJobs{}
	svc := &enqueueTestServices{}
	chat := &messenger.FakeChat{}

	ok := EnqueueFreeText(context.Background(), svc, jobs, chat, 7, "gasté 500", context.Canceled)
	if ok {
		t.Fatal("want false (not a 429)")
	}
	if len(jobs.inserted) != 0 {
		t.Errorf("un error no-429 no se encola, got %+v", jobs.inserted)
	}
}

func TestEnqueueBehindPending_WithPending_Enqueues(t *testing.T) {
	jobs := &fakeJobs{count: 1}
	svc := &enqueueTestServices{}
	chat := &messenger.FakeChat{}

	if !EnqueueBehindPending(context.Background(), svc, jobs, chat, 7, "uh no, eran 600") {
		t.Fatal("want enqueued (true)")
	}
	if len(jobs.inserted) != 1 || jobs.inserted[0].Kind != KindFreeText {
		t.Fatalf("want 1 free_text job, got %+v", jobs.inserted)
	}
}

func TestEnqueueBehindPending_NoPending_PassesThrough(t *testing.T) {
	jobs := &fakeJobs{count: 0}
	svc := &enqueueTestServices{}
	chat := &messenger.FakeChat{}

	if EnqueueBehindPending(context.Background(), svc, jobs, chat, 7, "gasté 500") {
		t.Fatal("want false (nothing pending → process live)")
	}
}

func TestEnqueueUpdatePick_RateLimited_Enqueues(t *testing.T) {
	jobs := &fakeJobs{}
	svc := &enqueueTestServices{}
	chat := &messenger.FakeChat{}
	err := &orchestrator.RateLimitedError{RetryAfter: 10 * time.Second}

	ok := EnqueueUpdatePick(context.Background(), svc, jobs, chat, 7, "msg", "tx1", []string{"a"}, nil, err)
	if !ok {
		t.Fatal("want enqueued (true)")
	}
	if len(jobs.inserted) != 1 || jobs.inserted[0].Kind != KindUpdatePick {
		t.Fatalf("want 1 update_pick job, got %+v", jobs.inserted)
	}
}

func TestEnqueueUpdatePick_NotRateLimited_NoEnqueue(t *testing.T) {
	jobs := &fakeJobs{}
	svc := &enqueueTestServices{}
	chat := &messenger.FakeChat{}

	ok := EnqueueUpdatePick(context.Background(), svc, jobs, chat, 7, "msg", "tx1", nil, nil, context.Canceled)
	if ok {
		t.Fatal("want false (not a 429)")
	}
	if len(jobs.inserted) != 0 {
		t.Errorf("un error no-429 no se encola, got %+v", jobs.inserted)
	}
}

func TestHandleGroqError_RateLimited_Enqueues(t *testing.T) {
	jobs := &fakeJobs{}
	svc := &enqueueTestServices{}
	chat := &messenger.FakeChat{}
	err := &orchestrator.RateLimitedError{RetryAfter: 5 * time.Second}

	handled, outErr := HandleGroqError(context.Background(), svc, jobs, chat, 7, "gasté 5000", err)
	if !handled {
		t.Fatal("want handled=true")
	}
	if outErr != nil {
		t.Fatalf("want nil outErr, got %v", outErr)
	}
	if len(jobs.inserted) != 1 {
		t.Fatalf("want 1 job, got %+v", jobs.inserted)
	}
}

func TestHandleGroqError_ReplayRateLimited_Propagates(t *testing.T) {
	jobs := &fakeJobs{}
	svc := &enqueueTestServices{}
	chat := &messenger.FakeChat{}
	err := &orchestrator.RateLimitedError{RetryAfter: 5 * time.Second}
	ctx := WithReplaying(context.Background())

	handled, outErr := HandleGroqError(ctx, svc, jobs, chat, 7, "text", err)
	if !handled {
		t.Fatal("want handled=true")
	}
	if outErr != err {
		t.Fatalf("want same error propagated, got %v", outErr)
	}
	if len(jobs.inserted) != 0 {
		t.Errorf("replay must not enqueue, got %+v", jobs.inserted)
	}
}

func TestHandleGroqError_NonRateLimited_NotHandled(t *testing.T) {
	jobs := &fakeJobs{}
	svc := &enqueueTestServices{}
	chat := &messenger.FakeChat{}

	handled, _ := HandleGroqError(context.Background(), svc, jobs, chat, 7, "text", context.Canceled)
	if handled {
		t.Fatal("want handled=false for non-429")
	}
}

func TestAckForWait_Short(t *testing.T) {
	got := AckForWait(5 * time.Second)
	if got != msgAckShortWait {
		t.Errorf("got %q, want %q", got, msgAckShortWait)
	}
}

func TestAckForWait_Long(t *testing.T) {
	got := AckForWait(120 * time.Second)
	want := "Estoy sin cupo por ~2 min 🙏 lo cargo apenas se libere y te aviso."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
