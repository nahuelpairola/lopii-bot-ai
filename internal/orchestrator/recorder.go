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

// LLMCall es el dato que send() emite por cada llamada Groq. Es el tipo del
// orquestador (no el modelo GORM de metric — regla cross-package); server mapea.
type LLMCall struct {
	TraceID                    string
	CallType                   string
	Model                      string
	PromptTokens               int
	CompletionTokens           int
	TotalTokens                int
	LatencyMs                  int
	HTTPStatus                 int
	Attempts                   int
	Err                        string
	RateLimitRemainingRequests *int
	RateLimitRemainingTokens   *int
}

// LLMRecorder recibe cada LLMCall. Implementado por un adapter en server que
// escribe a metric. Inyectado en el Client; nil-safe (tests no lo setean).
type LLMRecorder interface {
	Record(LLMCall)
}

// callType* etiquetan la llamada para agregados por tipo en Grafana.
const (
	callTypeRouter         = "router"
	callTypeCreate         = "create"
	callTypeUpdate         = "update"
	callTypeDelete         = "delete"
	callTypeOnboarding     = "onboarding"
	callTypeQuery          = "query"
	callTypeCategoryCreate = "category_create"
	callTypeAccountManage  = "account_manage"
)

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
