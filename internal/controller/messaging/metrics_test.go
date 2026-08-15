package messaging

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
)

func TestRouterOutcome(t *testing.T) {
	cases := map[orchestrator.Intent]string{
		orchestrator.IntentCreate:         outcomePending,
		orchestrator.IntentUpdate:         outcomePending,
		orchestrator.IntentDelete:         outcomePending,
		orchestrator.IntentQuery:          outcomePending,
		orchestrator.IntentAccountManage:  outcomePending,
		orchestrator.IntentCreateCategory: outcomePending,
	}
	for intent, want := range cases {
		if got := routerOutcome(intent); got != want {
			t.Errorf("routerOutcome(%q) = %q, want %q", intent, got, want)
		}
	}
}

// fakeMetricRepo captura las llamadas para las Tasks 4-6.
type fakeMetricRepo struct {
	logged        []loggedIntent
	resolved      []string
	resolvedIDs   [][]uint
	queuedIntents []string
}

type loggedIntent struct {
	intent  string
	outcome string
}

func (f *fakeMetricRepo) Log(userID uint64, traceID, rawMessage, intent string, needsConfirmation bool, outcome string) error {
	f.logged = append(f.logged, loggedIntent{intent: intent, outcome: outcome})
	return nil
}

func (f *fakeMetricRepo) Resolve(userID uint64, outcome string, movementIDs []uint) error {
	f.resolved = append(f.resolved, outcome)
	f.resolvedIDs = append(f.resolvedIDs, movementIDs)
	return nil
}

func TestResolveMetric_PassesMovementIDs(t *testing.T) {
	f := &fakeMetricRepo{}
	c := &controller{metrics: f}
	c.resolveMetric(context.Background(), 7, outcomeDeleteConfirmed, 71, 72)
	if len(f.resolvedIDs) != 1 || len(f.resolvedIDs[0]) != 2 || f.resolvedIDs[0][0] != 71 || f.resolvedIDs[0][1] != 72 {
		t.Fatalf("expected ids [71 72], got %v", f.resolvedIDs)
	}
}

// Una corrección que se desvía al flujo de alta para llenar un gap de categoría
// NO es un alta. Medido en vivo el 2026-08-12: "Era pollo" corrigió el
// movimiento y quedó como create_inserted, que es la columna que lee el portón
// de la etapa.
func TestWriteOutcomeFor(t *testing.T) {
	if got := writeOutcomeFor(conversation.Data{conversation.KeyMode: modeUpdate}); got != outcomeUpdateConfirmed {
		t.Errorf("modo update escribió %q, want %q", got, outcomeUpdateConfirmed)
	}
	if got := writeOutcomeFor(conversation.Data{}); got != outcomeCreateInserted {
		t.Errorf("sin modo escribió %q, want %q", got, outcomeCreateInserted)
	}
	if got := failureOutcomeFor(conversation.Data{conversation.KeyMode: modeUpdate}); got != outcomeWriteFailed {
		t.Errorf("falla en modo update escribió %q, want %q", got, outcomeWriteFailed)
	}
	if got := failureOutcomeFor(conversation.Data{}); got != outcomeCreateFailed {
		t.Errorf("falla sin modo escribió %q, want %q", got, outcomeCreateFailed)
	}
}

// Un 429 se encola y se replaya: el evento tiene que nacer PENDING, porque la
// historia no terminó. Si nace `unclear` —que es terminal— el replay exitoso no
// encuentra ningún pendiente que resolver, y el movimiento entra sin que la
// métrica lo registre.
//
// Medido en vivo el 2026-08-12: "Cobré 500000 de sueldo" insertó $500.000 y
// quedó contado como falla.
func TestInitialOutcome(t *testing.T) {
	rateLimited := &orchestrator.RateLimitedError{}

	if got := initialOutcome(orchestrator.IntentQueued, rateLimited); got != outcomePending {
		t.Errorf("con 429 el evento nace %q, want %q", got, outcomePending)
	}
	// Y el INTENT nace QUEUED, no UNCLEAR: el cupo cortó antes de que el modelo
	// eligiera herramienta, así que no es que no se entendió — no se intentó.
	if got := intentForExecutor(&agentExecutor{}, rateLimited); got != orchestrator.IntentQueued {
		t.Errorf("con 429 el intent es %q, want %q", got, orchestrator.IntentQueued)
	}
	if got := intentForExecutor(&agentExecutor{}, errors.New("transporte")); got != orchestrator.IntentUnclear {
		t.Errorf("un error que no es de cupo da %q, want UNCLEAR", got)
	}
	// Envuelto, que es como llega de verdad desde el loop.
	wrapped := fmt.Errorf("agent loop: %w", rateLimited)
	if got := initialOutcome(orchestrator.IntentQueued, wrapped); got != outcomePending {
		t.Errorf("con 429 envuelto nace %q, want %q", got, outcomePending)
	}
	// Sin error, o con uno que no es de cupo, manda el intent.
	if got := initialOutcome(orchestrator.IntentUnclear, nil); got != outcomeUnclear {
		t.Errorf("sin error nace %q, want %q", got, outcomeUnclear)
	}
	if got := initialOutcome(orchestrator.IntentCreate, errors.New("boom")); got != outcomePending {
		t.Errorf("un error que no es de cupo nace %q, want %q", got, outcomePending)
	}
}

// queuedIntents guarda las correcciones de intent que hizo el drenaje.
func (f *fakeMetricRepo) SetIntentIfQueued(userID uint64, intent string) error {
	f.queuedIntents = append(f.queuedIntents, intent)
	return nil
}
