package messaging

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingjob"
	"lopiibot.com/internal/trace"
	"lopiibot.com/internal/user"
)

type drainJobs struct {
	jobs     []pendingjob.PendingJob
	deleted  []uint64
	inserted []pendingjob.PendingJob
}

func (d *drainJobs) Insert(j *pendingjob.PendingJob) error {
	d.inserted = append(d.inserted, *j)
	return nil
}
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

// traceSpyOrch anota el trace_id que ve el replay. Es la aserción que importa:
// el ctx del drenaje viene del ticker del server, así que sin trace propio TODA
// llamada a Groq nacida de un mensaje encolado escribe llm_calls con trace_id
// vacío — y ese es justo el mensaje que uno quiere poder rastrear.
type traceSpyOrch struct {
	unclearOrch
	seen string
}

func (o *traceSpyOrch) ClassifyIntent(ctx context.Context, _ string) (orchestrator.IntentResult, error) {
	o.seen = trace.ID(ctx)
	return orchestrator.IntentResult{Intent: orchestrator.IntentUnclear}, nil
}

func TestDrain_ReplayCarriesItsOwnTrace(t *testing.T) {
	payload, _ := json.Marshal(freeTextPayload{Text: "la panaderia eran 2 mil"})
	jobs := &drainJobs{jobs: []pendingjob.PendingJob{
		{ID: 9, UserID: 7, Kind: kindFreeText, Payload: payload, CreatedAt: time.Now()},
	}}
	orch := &traceSpyOrch{}
	tr := &fakeTraceRepo{}
	c := &controller{jobs: jobs, users: fakeUsers{}, orchestrator: orch, traces: tr}

	// ctx pelado, como el del ticker: no trae trace.
	c.drainTick(context.Background(), nil, time.Now())

	if orch.seen == "" {
		t.Error("el replay corrió sin trace_id: llm_calls e intent_events quedan huérfanos")
	}
	if !tr.inserted {
		t.Fatal("el replay no dejó fila en request_traces")
	}
	if tr.traceID != orch.seen {
		t.Errorf("el spine dice %q y el replay usó %q — no se pueden joinear", tr.traceID, orch.seen)
	}
	if tr.updType != updateTypeReplay {
		t.Errorf("update_type = %q, want %q", tr.updType, updateTypeReplay)
	}
	if tr.userID == nil || *tr.userID != 7 {
		t.Errorf("user_id = %v, want 7", tr.userID)
	}
}
