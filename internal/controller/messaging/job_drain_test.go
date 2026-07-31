package messaging

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/user"
)

type drainJobs struct {
	jobs    []pendingjob.PendingJob
	deleted []uint64
}

func (d *drainJobs) Insert(*pendingjob.PendingJob) error                       { return nil }
func (d *drainJobs) ListByUserOrdered(uint64) ([]pendingjob.PendingJob, error) { return d.jobs, nil }
func (d *drainJobs) ListPendingUserIDs() ([]uint64, error)                     { return []uint64{7}, nil }
func (d *drainJobs) Delete(id uint64) error                                    { d.deleted = append(d.deleted, id); return nil }
func (d *drainJobs) CountByUser(uint64) (int64, error)                         { return int64(len(d.jobs)), nil }

type fakeUsers struct{}

func (fakeUsers) FindByTelegramID(string) (*user.User, error) { return nil, nil }
func (fakeUsers) FindByID(uint64) (*user.User, error)         { return &user.User{TelegramID: "100"}, nil }
func (fakeUsers) Insert(*user.User) error                     { return nil }

func TestDrain_GiveUp_OldJob(t *testing.T) {
	payload, _ := json.Marshal(freeTextPayload{Text: "gasté 5000"})
	jobs := &drainJobs{jobs: []pendingjob.PendingJob{
		{ID: 42, UserID: 7, Kind: kindFreeText, Payload: payload, CreatedAt: time.Now().Add(-3 * time.Hour)},
	}}
	// orchestrator nil: si el drain lo invocara, panichearía → prueba que NO lo invoca.
	c := &controller{jobs: jobs, users: fakeUsers{}}

	c.drainTick(context.Background(), nil, time.Now())

	if len(jobs.deleted) != 1 || jobs.deleted[0] != 42 {
		t.Fatalf("want job 42 deleted, got %v", jobs.deleted)
	}
}

func TestDrain_Success_Deletes(t *testing.T) {
	// job fresco (no give-up); orchestrator stub que rutea a UNCLEAR (no toca red real).
	payload, _ := json.Marshal(freeTextPayload{Text: "hola"})
	jobs := &drainJobs{jobs: []pendingjob.PendingJob{
		{ID: 9, UserID: 7, Kind: kindFreeText, Payload: payload, CreatedAt: time.Now()},
	}}
	c := &controller{jobs: jobs, users: fakeUsers{}, orchestrator: unclearOrch{}}
	c.drainTick(context.Background(), nil, time.Now())
	if len(jobs.deleted) != 1 || jobs.deleted[0] != 9 {
		t.Fatalf("want job 9 deleted, got %v", jobs.deleted)
	}
}

// unclearOrch clasifica todo como UNCLEAR (handleFreeText responde msgAskRewrite,
// no toca Call 2). Embeds movementOrchestrator para satisfacer la interfaz.
type unclearOrch struct{ movementOrchestrator }

func (unclearOrch) ClassifyIntent(context.Context, string) (orchestrator.IntentResult, error) {
	return orchestrator.IntentResult{Intent: orchestrator.IntentUnclear}, nil
}
