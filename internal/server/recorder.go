package server

import (
	"log/slog"

	"lopiibot.com/internal/metric"
	"lopiibot.com/internal/orchestrator"
)

type llmCallRecorder struct {
	insert func(*metric.LLMCall) error
}

func (r llmCallRecorder) Record(c orchestrator.LLMCall) {
	go func() {
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
