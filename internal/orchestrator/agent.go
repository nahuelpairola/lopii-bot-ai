package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

func toolCallNames(calls []loopToolCall) []string {
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.Function.Name)
	}
	return names
}

const maxAgentIterations = 5

var ErrAgentMaxIterations = errors.New("orchestrator: agent loop exceeded max iterations")

var ErrAgentTurnDone = errors.New("orchestrator: agent turn done")

func (o *Orchestrator) Run(ctx context.Context, systemPrompt, userText string, history []QueryTurn, tools []AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	toolDefs := make([]toolDef, len(tools))
	for i, t := range tools {
		toolDefs[i] = toolDef{
			Type:     "function",
			Function: toolFunction{Name: t.Name, Description: t.Description, Parameters: t.Parameters},
		}
	}

	messages := []loopMessage{{Role: "system", Content: systemPrompt}}
	for _, t := range history {
		messages = append(messages,
			loopMessage{Role: "user", Content: t.Question},
			loopMessage{Role: "assistant", Content: t.Answer},
		)
	}
	messages = append(messages, loopMessage{Role: "user", Content: userText})

	for i := 0; i < maxAgentIterations; i++ {
		choice := "auto"
		if i == 0 {
			choice = "required"
		}
		assistant, err := o.agentRound(ctx, messages, toolDefs, choice)
		if errors.Is(err, ErrNothingToExtract) && choice == "required" {
			assistant, err = o.agentRound(ctx, messages, toolDefs, "auto")
		}
		if err != nil {
			return "", fmt.Errorf("orchestrator: agent run: %w", err)
		}

		slog.InfoContext(ctx, "agent round",
			"round", i,
			"tools", toolCallNames(assistant.ToolCalls),
			"narrated", strings.TrimSpace(assistant.Content) != "",
		)

		if len(assistant.ToolCalls) == 0 {
			return assistant.Content, nil
		}

		messages = append(messages, assistant)
		turnDone := false
		for _, call := range assistant.ToolCalls {
			result, execErr := execute(call.Function.Name, json.RawMessage(call.Function.Arguments))
			switch {
			case errors.Is(execErr, ErrAgentTurnDone):
				turnDone = true
			case execErr != nil:
				slog.WarnContext(ctx, "agent tool failed",
					"round", i, "tool", call.Function.Name, "err", execErr)
				result = fmt.Sprintf("error: %v", execErr)
			}
			messages = append(messages, loopMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
		}

		if turnDone || strings.TrimSpace(assistant.Content) != "" {
			return assistant.Content, nil
		}
	}

	final, err := o.agentRound(ctx, messages, nil, "none")
	if err != nil {
		return "", fmt.Errorf("orchestrator: agent run (final): %w", err)
	}
	if strings.TrimSpace(final.Content) == "" {
		return "", ErrAgentMaxIterations
	}
	return final.Content, nil
}

func (o *Orchestrator) agentRound(ctx context.Context, messages []loopMessage, tools []toolDef, toolChoice string) (loopMessage, error) {
	chain := append([]string{o.agentModel}, o.agentFallbacks...)
	return o.roundWithFallback(ctx, callTypeAgent, chain, messages, tools, toolChoice, maxAgentCompletionTokens)
}

func (o *Orchestrator) roundWithFallback(ctx context.Context, callType string, chain []string,
	messages []loopMessage, tools []toolDef, toolChoice string, maxTokens int) (loopMessage, error) {
	var lastErr error
	for i, model := range chain {
		msg, err := o.client.chatCompletionLoop(ctx, callType, model, messages, tools, toolChoice, maxTokens)
		if err == nil {
			return msg, nil
		}
		var rateLimited *RateLimitedError
		if !errors.As(err, &rateLimited) {
			return msg, err
		}
		lastErr = err
		slog.WarnContext(ctx, "modelo sin cupo, probando el siguiente",
			"call_type", callType, "model", model, "restantes", len(chain)-i-1)
	}
	return loopMessage{}, lastErr
}
