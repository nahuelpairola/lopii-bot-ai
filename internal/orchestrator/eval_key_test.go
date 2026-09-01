//go:build llm_eval

package orchestrator

import (
	"os"
	"testing"
)

func evalKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("GROQ_APIKEY")
	if key == "" {
		key = os.Getenv("GROQ_API_KEY")
	}
	if key == "" {
		t.Fatal("GROQ_APIKEY unset — the llm_eval tag was requested on purpose, so skipping would be a green that proves nothing. Export it from .env.")
	}
	return key
}

func evalBaseURL() string {
	if u := os.Getenv("GROQ_BASE_URL"); u != "" {
		return u
	}
	return "https://api.groq.com/openai/v1"
}
