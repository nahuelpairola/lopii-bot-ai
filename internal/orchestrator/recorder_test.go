package orchestrator

import (
	"net/http"
	"testing"
)

func TestParseUsage(t *testing.T) {
	body := []byte(`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)
	p, c, tot := parseUsage(body)
	if p != 11 || c != 7 || tot != 18 {
		t.Fatalf("got %d/%d/%d, want 11/7/18", p, c, tot)
	}
	if p, c, tot := parseUsage([]byte(`{}`)); p != 0 || c != 0 || tot != 0 {
		t.Fatalf("empty usage: got %d/%d/%d", p, c, tot)
	}
}

func TestParseRateLimitRemaining(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-remaining-requests", "199")
	h.Set("x-ratelimit-remaining-tokens", "48000")
	req, tok := parseRateLimitRemaining(h)
	if req == nil || *req != 199 || tok == nil || *tok != 48000 {
		t.Fatalf("got %v/%v", req, tok)
	}
	if req, tok := parseRateLimitRemaining(http.Header{}); req != nil || tok != nil {
		t.Fatalf("absent headers must be nil, got %v/%v", req, tok)
	}
}
