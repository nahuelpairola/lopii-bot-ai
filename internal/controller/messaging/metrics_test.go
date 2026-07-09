package messaging

import (
	"testing"

	"lopiibot.com/internal/orchestrator"
)

func TestRouterOutcome(t *testing.T) {
	cases := map[orchestrator.Intent]string{
		orchestrator.IntentCreate:         outcomePending,
		orchestrator.IntentUpdate:         outcomePending,
		orchestrator.IntentDelete:         outcomePending,
		orchestrator.IntentQuery:          outcomePending,
		orchestrator.IntentAccountCreate:  outcomeAccountCreateRouted,
		orchestrator.IntentCreateCategory: outcomeCategoryCreateRouted,
	}
	for intent, want := range cases {
		if got := routerOutcome(intent); got != want {
			t.Errorf("routerOutcome(%q) = %q, want %q", intent, got, want)
		}
	}
}

// fakeMetricRepo captura las llamadas para las Tasks 4-6.
type fakeMetricRepo struct {
	logged      []loggedIntent
	resolved    []string
	resolvedIDs [][]uint
}

type loggedIntent struct {
	intent  string
	outcome string
}

func (f *fakeMetricRepo) Log(userID uint64, rawMessage, intent string, needsConfirmation bool, outcome string) error {
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
	c.resolveMetric(7, outcomeDeleteConfirmed, 71, 72)
	if len(f.resolvedIDs) != 1 || len(f.resolvedIDs[0]) != 2 || f.resolvedIDs[0][0] != 71 || f.resolvedIDs[0][1] != 72 {
		t.Fatalf("expected ids [71 72], got %v", f.resolvedIDs)
	}
}
