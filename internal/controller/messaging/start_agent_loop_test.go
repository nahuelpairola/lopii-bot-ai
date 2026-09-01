package messaging

import (
	"context"
	"encoding/json"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
)

func newLoopController(t *testing.T, orch *fakeFullOrchestrator, repo *fakeActionsRepo, movements *fakeMovementRepoFull) *controller {
	t.Helper()
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(flow.NewAskUserFlow())
	engine.Register(flow.NewMovementDeleteFlow())
	engine.Register(flow.NewMovementUpdateConfirmFlow())
	return &controller{
		engine:        engine,
		orchestrator:  orch,
		actions:       repo,
		movements:     movements,
		accounts:      &fakeAccountRepoFull{},
		subcategories: &fakeSubcategoryRepoFull{},
		chatHistory:   stubChatHistory{},
		metrics:       &fakeMetricRepo{},
	}
}

func TestRouting_EverythingGoesThroughTheLoop(t *testing.T) {
	for _, msg := range []string{"Cafe 12700", "¿cuánto gasté en julio?", "editá los del lote", "renombrá Galicia", "hola", "asdkjhasd"} {
		t.Run(msg, func(t *testing.T) {
			called := false
			orch := &fakeFullOrchestrator{
				runFn: func(func(string, json.RawMessage) (string, error)) (string, error) {
					called = true
					return "listo", nil
				},
			}
			c := newLoopController(t, orch, &fakeActionsRepo{}, &fakeMovementRepoFull{})

			if err := c.handleFreeText(context.Background(), &messenger.FakeChat{}, 1, msg); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatalf("%q tiene que ir por el agent loop", msg)
			}
		})
	}
}
