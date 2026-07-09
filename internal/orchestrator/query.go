package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// AgentTool is a tool exposed to the query loop. Same shape as toolSchema,
// but public so the messaging controller can define the read tools and hold
// the executor — the orchestrator stays dependency-free (no repo imports).
type AgentTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// maxQueryIterations caps how many tool rounds the loop runs before giving
// up. Typed tools resolve fast; a well-behaved query is 1 tool round + 1
// narration. The cap is a runaway guard, not the expected path.
const maxQueryIterations = 3

var ErrQueryMaxIterations = errors.New("orchestrator: query loop exceeded max iterations")

// AnswerQuery runs the read-only agent loop: it sends the tools with
// tool_choice:"auto", executes every tool call the model emits in a round
// (via the caller's execute closure, scoped to the user), feeds each result
// back as a tool message, and repeats until the model returns content
// instead of tool calls — that content is the final Spanish answer. A tool
// executor error is fed back to the model as an error string, not aborted,
// so the model can recover or explain. If the cap is reached while the model
// still wants tools, one final tool_choice:"none" call forces a narration
// from the accumulated results (only a truly empty final response yields
// ErrQueryMaxIterations).
func (o *Orchestrator) AnswerQuery(ctx context.Context, systemPrompt, userText string, tools []AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	toolDefs := make([]toolDef, len(tools))
	for i, t := range tools {
		toolDefs[i] = toolDef{
			Type:     "function",
			Function: toolFunction{Name: t.Name, Description: t.Description, Parameters: t.Parameters},
		}
	}

	messages := []loopMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userText},
	}

	for i := 0; i < maxQueryIterations; i++ {
		// Force a tool call on the first round: weak models (8b-instant)
		// sometimes deflect ("no puedo darte una respuesta exacta") without
		// ever calling a tool. "required" guarantees the loop gathers real
		// data before it is allowed to narrate; later rounds go back to "auto".
		choice := "auto"
		if i == 0 {
			choice = "required"
		}
		assistant, err := o.client.chatCompletionLoop(ctx, o.queryModel, messages, toolDefs, choice)
		if err != nil {
			return "", fmt.Errorf("orchestrator: answer query: %w", err)
		}
		if len(assistant.ToolCalls) == 0 {
			return assistant.Content, nil
		}
		messages = append(messages, assistant)
		for _, call := range assistant.ToolCalls {
			result, execErr := execute(call.Function.Name, json.RawMessage(call.Function.Arguments))
			if execErr != nil {
				result = fmt.Sprintf("error: %v", execErr)
			}
			messages = append(messages, loopMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
		}
	}

	// Cap reached while the model still wanted tools. Force one final
	// narration (tool_choice:"none") so it summarizes from the tool results
	// it already has instead of failing — Groq's documented way to guarantee
	// text over another tool round. 8b-instant is weak at deciding to stop on
	// its own, so this is a common path to a clean answer, not a rare one.
	final, err := o.client.chatCompletionLoop(ctx, o.queryModel, messages, toolDefs, "none")
	if err != nil {
		return "", fmt.Errorf("orchestrator: answer query (final): %w", err)
	}
	if strings.TrimSpace(final.Content) == "" {
		return "", ErrQueryMaxIterations
	}
	return final.Content, nil
}
