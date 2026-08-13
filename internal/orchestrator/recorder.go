package orchestrator

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
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

// parseToolCalls devuelve el array tool_calls de la respuesta, tal cual, como
// JSON. Sin esto llm_calls guarda QUÉ tool eligió el modelo pero nunca CON QUÉ
// argumentos, y medir la selección sin los argumentos es medir media decisión.
//
// Vacío cuando el modelo no llamó nada, que server mapea a NULL: así
// `WHERE tool_calls IS NOT NULL` significa "llamó algo".
func parseToolCalls(body []byte) string {
	var r struct {
		Choices []struct {
			Message struct {
				ToolCalls json.RawMessage `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &r); err != nil || len(r.Choices) == 0 {
		return ""
	}
	raw := r.Choices[0].Message.ToolCalls
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return ""
	}
	return string(raw)
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
	// ToolCalls: el array tool_calls crudo. Vacío = el modelo no llamó nada.
	ToolCalls string
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
	// callTypeClassifier: la clasificación sale del loop y pasa a su propia
	// llamada, en su propio bucket de Grafana y —lo que importa— en su propio
	// techo de TPM, porque el ceiling de Groq es POR MODELO.
	callTypeClassifier    = "classifier"
	callTypeAccountManage = "account_manage"
	// callTypeAgent es el loop unificado (Run). Va aparte de callTypeQuery a
	// propósito: desde la etapa 2 el loop atiende UPDATE y DELETE, y si compartiera
	// bucket con query los paneles de costo y latencia de QUERY empezarían a
	// mezclar tráfico que no es de consultas, sin que nadie se entere.
	callTypeAgent = "agent"
)

// parseResetTokens lee el header x-ratelimit-reset-tokens de Groq (string de
// duración, e.g. "16m29.28s"), el reset confiable para límites por token
// (TPM/TPD). 0 si ausente o no parseable.
func parseResetTokens(h http.Header) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(h.Get("x-ratelimit-reset-tokens")))
	if err != nil || d <= 0 {
		return 0
	}
	return d
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
