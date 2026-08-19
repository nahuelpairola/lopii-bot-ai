package agent

import (
	"errors"
	"fmt"
	"testing"

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
