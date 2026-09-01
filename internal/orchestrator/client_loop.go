package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type loopToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function loopToolCallFunc `json:"function"`
}

type loopToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type loopMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []loopToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type loopRequest struct {
	Model               string        `json:"model"`
	Messages            []loopMessage `json:"messages"`
	Tools               []toolDef     `json:"tools"`
	ToolChoice          string        `json:"tool_choice"`
	Temperature         float64       `json:"temperature"`
	MaxCompletionTokens int           `json:"max_completion_tokens,omitempty"`
	ReasoningEffort     string        `json:"reasoning_effort,omitempty"`
}

func lowReasoningEffort(model string) string {
	if strings.HasPrefix(model, "openai/gpt-oss") {
		return "low"
	}
	return ""
}

const maxFirstRoundCompletionTokens = 640

const maxQueryCompletionTokens = 1024

const maxNarrationCompletionTokens = 400

const maxAgentCompletionTokens = 1500

type loopResponse struct {
	Choices []struct {
		Message loopMessage `json:"message"`
	} `json:"choices"`
}

func (c *Client) chatCompletionLoop(ctx context.Context, callType, model string, messages []loopMessage, tools []toolDef, toolChoice string, maxTokens int) (loopMessage, error) {
	reqBody := loopRequest{
		Model:               model,
		Messages:            messages,
		Tools:               tools,
		ToolChoice:          toolChoice,
		Temperature:         0.1,
		MaxCompletionTokens: maxTokens,
	}
	if toolChoice == "none" {
		reqBody.ReasoningEffort = lowReasoningEffort(model)
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return loopMessage{}, fmt.Errorf("orchestrator: marshal loop request: %w", err)
	}

	body, err := c.send(ctx, callType, model, payload)
	if err != nil {
		return loopMessage{}, err
	}

	var parsed loopResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return loopMessage{}, fmt.Errorf("orchestrator: unmarshal loop response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return loopMessage{}, fmt.Errorf("orchestrator: groq loop response had no choices")
	}
	return parsed.Choices[0].Message, nil
}
