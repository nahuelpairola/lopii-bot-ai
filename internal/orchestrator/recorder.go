package orchestrator

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// parseUsage extrae el bloque usage estándar (OpenAI-compatible) del body de
// Groq. Tolera ausencia (error, forma inesperada) → ceros, nunca panic.
func parseUsage(body []byte) (prompt, completion, total int) {
	var u struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(body, &u)
	return u.Usage.PromptTokens, u.Usage.CompletionTokens, u.Usage.TotalTokens
}

// parseRateLimitRemaining lee los headers x-ratelimit-remaining-* de Groq.
// Ausente / no numérico → nil (columna queda NULL).
func parseRateLimitRemaining(h http.Header) (*int, *int) {
	return atoiPtr(h.Get("x-ratelimit-remaining-requests")),
		atoiPtr(h.Get("x-ratelimit-remaining-tokens"))
}

func atoiPtr(s string) *int {
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &n
}
