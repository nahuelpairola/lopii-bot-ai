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
		orchestrator.IntentQuery:          outcomeQueryUnsupported,
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
	logged   []loggedIntent
	resolved []string
}

type loggedIntent struct {
	intent  string
	outcome string
}

func (f *fakeMetricRepo) Log(userID uint64, rawMessage, intent string, needsConfirmation bool, outcome string) error {
	f.logged = append(f.logged, loggedIntent{intent: intent, outcome: outcome})
	return nil
}

func (f *fakeMetricRepo) Resolve(userID uint64, outcome string) error {
	f.resolved = append(f.resolved, outcome)
	return nil
}
