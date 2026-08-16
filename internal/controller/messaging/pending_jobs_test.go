package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"lopiibot.com/internal/agent"
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

// orchestrator stub que siempre devuelve RateLimitedError en el loop.
type rateLimitedOrch struct{ movementOrchestrator }

// El 429 ahora aparece en el loop, no en el router: es la PRIMERA llamada del
// turno desde que handleFreeText no clasifica nada.
func (rateLimitedOrch) Run(context.Context, string, string, []orchestrator.QueryTurn, []orchestrator.AgentTool, func(string, json.RawMessage) (string, error)) (string, error) {
	return "", &orchestrator.RateLimitedError{RetryAfter: 8 * time.Second}
}

func TestHandleFreeText_RateLimited_Enqueues(t *testing.T) {
	jobs := &fakeJobs{}
	c := &controller{orchestrator: rateLimitedOrch{}, jobs: jobs,
		accounts: &fakeAccountRepoFull{}, subcategories: &fakeSubcategoryRepoFull{},
		movements: &fakeMovementRepoFull{}, chatHistory: stubChatHistory{}}

	err := c.handleFreeText(context.Background(), nil, 100, 7, "gasté 5000 en el super")
	if err != nil {
		t.Fatalf("want nil (enqueued, not error), got %v", err)
	}
	if len(jobs.inserted) != 1 || jobs.inserted[0].Kind != kindFreeText {
		t.Fatalf("want 1 free_text job, got %+v", jobs.inserted)
	}
}

// jobText devuelve el texto que viajó en un job free_text.
func jobText(t *testing.T, j pendingjob.PendingJob) string {
	t.Helper()
	var p freeTextPayload
	if err := json.Unmarshal(j.Payload, &p); err != nil {
		t.Fatalf("payload del job: %v", err)
	}
	return p.Text
}

// Regresión del 2026-08-10: cuatro consultas seguidas murieron con 429 de Groq y
// pending_llm_jobs quedó vacía — QUERY era el único camino que no pasaba por
// handleGroqError, así que el mensaje se perdía en vez de encolarse. Un 429 en
// QUERY es el más seguro de reintentar: es read-only, no puede duplicar plata.
//
// El bot es nil en los tests, así que sendText no manda nada y la copy no se puede
// observar; lo que se afirma acá es lo que sí se puede ver, que es lo que importa:
// el mensaje quedó guardado y la métrica no se cerró como fracaso.
func TestHandleFreeText_QueryRateLimited_EnqueuesInsteadOfLosingIt(t *testing.T) {
	// Sin router, la consulta llega al loop y el loop llama answer_query; el 429
	// aparece adentro del loop de query, que es donde sigue viviendo QUERY.
	orch := &fakeFullOrchestrator{
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			_, err := execute(orchestrator.ToolAnswerQuery, json.RawMessage(`{}`))
			if errors.Is(err, orchestrator.ErrAgentTurnDone) {
				return "", nil
			}
			return "", err
		},
		queryErr: &orchestrator.RateLimitedError{RetryAfter: 15 * time.Second},
	}
	jobs := &fakeJobs{}
	metrics := &fakeMetricRepo{}
	c := &controller{orchestrator: orch, metrics: metrics, chatHistory: stubChatHistory{}, jobs: jobs,
		accounts: &fakeAccountRepoFull{}, subcategories: &fakeSubcategoryRepoFull{}, movements: &fakeMovementRepoFull{}}

	const msg = "Cuanto gaste en lote, hbo y disney este mes?"
	if err := c.handleFreeText(context.Background(), nil, 123, 7, msg); err != nil {
		t.Fatalf("want nil (encolado, no es un error), got %v", err)
	}

	if len(jobs.inserted) != 1 || jobs.inserted[0].Kind != kindFreeText {
		t.Fatalf("want 1 job free_text, got %+v", jobs.inserted)
	}
	if got := jobText(t, jobs.inserted[0]); got != msg {
		t.Errorf("texto encolado = %q, want %q", got, msg)
	}
	for _, o := range metrics.resolved {
		if o == outcomeQueryFailed {
			t.Errorf("un 429 encolado no puede resolver %q: la historia la termina el drain", outcomeQueryFailed)
		}
	}
}

// La contracara: un error que NO es 429 sigue siendo un fracaso de verdad. Sin
// esta guarda, el fix de arriba podría tragarse todos los errores.
func TestHandleFreeText_QueryNonRateLimitError_FailsAndDoesNotEnqueue(t *testing.T) {
	orch := &fakeFullOrchestrator{
		runFn: func(execute func(string, json.RawMessage) (string, error)) (string, error) {
			_, err := execute(orchestrator.ToolAnswerQuery, json.RawMessage(`{}`))
			if errors.Is(err, orchestrator.ErrAgentTurnDone) {
				return "", nil
			}
			return "", err
		},
		queryErr: context.Canceled,
	}
	jobs := &fakeJobs{}
	metrics := &fakeMetricRepo{}
	c := &controller{orchestrator: orch, metrics: metrics, chatHistory: stubChatHistory{}, jobs: jobs,
		accounts: &fakeAccountRepoFull{}, subcategories: &fakeSubcategoryRepoFull{}, movements: &fakeMovementRepoFull{}}

	c.handleFreeText(context.Background(), nil, 123, 7, "consulta que falla")

	if len(jobs.inserted) != 0 {
		t.Errorf("un error no-429 no se encola, got %+v", jobs.inserted)
	}
	found := false
	for _, o := range metrics.resolved {
		if o == outcomeQueryFailed {
			found = true
		}
	}
	if !found {
		t.Errorf("want un resolve de %q, got %v", outcomeQueryFailed, metrics.resolved)
	}
}

// CREATE_CATEGORY trataba cualquier error de Groq como "no pude clasificar" y caía
// al wizard de 7 pasos. Con un 429 eso es peor que un error: el usuario cree que no
// lo entendieron y se come siete preguntas por un problema de cupo.
func TestCreateCategory_RateLimited_EnqueuesAndSkipsWizard(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{}
	orch := &fakeFullOrchestrator{
		runFn:       manageSettingsRun(agent.SettingsAreaCategory),
		categoryErr: &orchestrator.RateLimitedError{RetryAfter: 15 * time.Second},
	}
	jobs := &fakeJobs{}
	c, store := newCreateCategoryController(orch, subs)
	c.jobs = jobs

	const msg = "quiero una categoría proyecto hogar"
	if err := c.handleFreeText(context.Background(), nil, 0, 1, msg); err != nil {
		t.Fatalf("want nil (encolado), got %v", err)
	}

	if len(jobs.inserted) != 1 || jobs.inserted[0].Kind != kindFreeText {
		t.Fatalf("want 1 job free_text, got %+v", jobs.inserted)
	}
	if got := jobText(t, jobs.inserted[0]); got != msg {
		t.Errorf("texto encolado = %q, want %q", got, msg)
	}
	if store.found {
		t.Errorf("un 429 no puede arrancar el wizard: se encoló el mensaje, flow = %q", store.flowName)
	}
}

func TestOrderingInvariant_EnqueuesBehindPending(t *testing.T) {
	jobs := &fakeJobs{count: 1} // ya hay un pending
	c := &controller{jobs: jobs}

	if !c.enqueueBehindPending(context.Background(), nil, 100, 7, "uh no, eran 600") {
		t.Fatal("want enqueued (true)")
	}
	if len(jobs.inserted) != 1 || jobs.inserted[0].Kind != kindFreeText {
		t.Fatalf("want 1 free_text job, got %+v", jobs.inserted)
	}
}

func TestOrderingInvariant_NoPending_PassesThrough(t *testing.T) {
	jobs := &fakeJobs{count: 0}
	c := &controller{jobs: jobs}
	if c.enqueueBehindPending(context.Background(), nil, 100, 7, "gasté 500") {
		t.Fatal("want false (nothing pending → process live)")
	}
}
