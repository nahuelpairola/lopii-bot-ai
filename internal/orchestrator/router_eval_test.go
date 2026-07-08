//go:build llm_eval

package orchestrator

import (
	"context"
	"os"
	"testing"
)

// Run with real Groq creds:
//   GROQ_API_KEY=... GROQ_BASE_URL=... GROQ_ROUTER_MODEL=... go test -tags llm_eval ./internal/orchestrator/ -run TestRouterEval -v
// Excluded from the default `go test ./...` (no tag) so CI needs no API key.
//
// This table is a living seed, not a closed list: every time a real
// misclassification surfaces in production, add the actual message here
// as a permanent regression case.
var routerEvalCases = []struct {
	msg  string
	want Intent
}{
	{"En realidad rescate 5 mil del fci", IntentUpdate},
	{"el café en realidad era 3000", IntentUpdate},
	{"rescaté 100k de FCI", IntentCreate},
	{"me dieron 500 de aguinaldo", IntentCreate},
}

func TestRouterEval(t *testing.T) {
	key := os.Getenv("GROQ_API_KEY")
	if key == "" {
		t.Skip("GROQ_API_KEY unset — real-LLM eval skipped")
	}
	o := New(Config{
		APIKey:         key,
		BaseURL:        os.Getenv("GROQ_BASE_URL"),
		RouterModel:    os.Getenv("GROQ_ROUTER_MODEL"),
		TimeoutSeconds: 30,
	})
	for _, tc := range routerEvalCases {
		t.Run(tc.msg, func(t *testing.T) {
			res, err := o.ClassifyIntent(context.Background(), tc.msg)
			if err != nil {
				t.Fatalf("ClassifyIntent: %v", err)
			}
			if res.Intent != tc.want {
				t.Errorf("%q → intent %v, want %v", tc.msg, res.Intent, tc.want)
			}
		})
	}
}
