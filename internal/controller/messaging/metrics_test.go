package messaging

import (
	"context"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

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

const modeUpdate = flow.ModeUpdate

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

func (f *fakeMetricRepo) SetIntentIfQueued(userID uint64, intent string) error {
	f.queuedIntents = append(f.queuedIntents, intent)
	return nil
}
