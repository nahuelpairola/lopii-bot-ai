package messaging

import (
	"context"
	"testing"
	"time"

	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingjob"
)

type fakeJobs struct {
	inserted []pendingjob.PendingJob
	count    int64
}

func (f *fakeJobs) Insert(j *pendingjob.PendingJob) error {
	f.inserted = append(f.inserted, *j)
	return nil
}
func (f *fakeJobs) ListByUserOrdered(uint64) ([]pendingjob.PendingJob, error) { return f.inserted, nil }
func (f *fakeJobs) ListPendingUserIDs() ([]uint64, error)                     { return nil, nil }
func (f *fakeJobs) Delete(uint64) error                                       { return nil }
func (f *fakeJobs) CountByUser(uint64) (int64, error)                         { return f.count, nil }

// orchestrator stub que siempre devuelve RateLimitedError en ClassifyIntent.
type rateLimitedOrch struct{ movementOrchestrator }

func (rateLimitedOrch) ClassifyIntent(context.Context, string) (orchestrator.IntentResult, error) {
	return orchestrator.IntentResult{}, &orchestrator.RateLimitedError{RetryAfter: 8 * time.Second}
}

func TestHandleFreeText_RateLimited_Enqueues(t *testing.T) {
	jobs := &fakeJobs{}
	c := &controller{orchestrator: rateLimitedOrch{}, jobs: jobs}

	err := c.handleFreeText(context.Background(), nil, 100, 7, "gasté 5000 en el super")
	if err != nil {
		t.Fatalf("want nil (enqueued, not error), got %v", err)
	}
	if len(jobs.inserted) != 1 || jobs.inserted[0].Kind != kindFreeText {
		t.Fatalf("want 1 free_text job, got %+v", jobs.inserted)
	}
}
