package server

import (
	"log/slog"

	"lopiibot.com/internal/metric"
	"lopiibot.com/internal/orchestrator"
)

// llmCallRecorder adapta orchestrator.LLMRecorder a metric.
//
// Vive acá y no en orchestrator porque orchestrator es repo-free por diseño: no
// puede importar la base. Declara la interfaz y alguien de afuera la implementa;
// ese alguien es este adapter, cableado en buildOrchestrator. Sin él las
// llamadas a Groq salen igual, pero no queda rastro de ninguna.
//
// Lo que se pierde sin esto es todo el panel de observabilidad: tokens
// consumidos, cupo restante, reintentos y latencia por modelo, y —el más
// caro de reponer— qué modelo entró de respaldo después de un 429.
type llmCallRecorder struct {
	insert func(*metric.LLMCall) error
}

// Record es fire-and-forget en goroutine: la métrica no debe agregar latencia
// ni romper el flujo del usuario, así que el error sólo se loguea. El precio
// aceptado es que un corte del proceso pierde los registros en vuelo.
func (r llmCallRecorder) Record(c orchestrator.LLMCall) {
	go func() {
		// Vacío → NULL, para que `WHERE tool_calls IS NOT NULL` signifique "el
		// modelo llamó algo" y no "la columna trae un array vacío".
		var toolCalls *string
		if c.ToolCalls != "" {
			toolCalls = &c.ToolCalls
		}
		if err := r.insert(&metric.LLMCall{
			TraceID:                    c.TraceID,
			CallType:                   c.CallType,
			Model:                      c.Model,
			PromptTokens:               c.PromptTokens,
			CompletionTokens:           c.CompletionTokens,
			TotalTokens:                c.TotalTokens,
			LatencyMs:                  c.LatencyMs,
			HTTPStatus:                 c.HTTPStatus,
			Attempts:                   c.Attempts,
			Error:                      c.Err,
			RateLimitRemainingRequests: c.RateLimitRemainingRequests,
			RateLimitRemainingTokens:   c.RateLimitRemainingTokens,
			ToolCalls:                  toolCalls,
		}); err != nil {
			slog.Error("llm_call insert failed", "err", err)
		}
	}()
}
