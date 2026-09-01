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

func TestInitialOutcome(t *testing.T) {
	rateLimited := &orchestrator.RateLimitedError{}

	if got := initialOutcome(orchestrator.IntentQueued, rateLimited); got != outcomePending {
		t.Errorf("con 429 el evento nace %q, want %q", got, outcomePending)
	}
	if got := intentForExecutor(&agentExecutor{}, rateLimited); got != orchestrator.IntentQueued {
		t.Errorf("con 429 el intent es %q, want %q", got, orchestrator.IntentQueued)
	}
	if got := intentForExecutor(&agentExecutor{}, errors.New("transporte")); got != orchestrator.IntentUnclear {
		t.Errorf("un error que no es de cupo da %q, want UNCLEAR", got)
	}
	wrapped := fmt.Errorf("agent loop: %w", rateLimited)
	if got := initialOutcome(orchestrator.IntentQueued, wrapped); got != outcomePending {
		t.Errorf("con 429 envuelto nace %q, want %q", got, outcomePending)
	}
	if got := initialOutcome(orchestrator.IntentUnclear, nil); got != outcomeUnclear {
		t.Errorf("sin error nace %q, want %q", got, outcomeUnclear)
	}
	if got := initialOutcome(orchestrator.IntentCreate, errors.New("boom")); got != outcomePending {
		t.Errorf("un error que no es de cupo nace %q, want %q", got, outcomePending)
	}
}
