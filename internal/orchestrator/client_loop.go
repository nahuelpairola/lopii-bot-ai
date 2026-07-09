package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// loopToolCall is one tool call inside an assistant message during the
// multi-tool agent loop.
type loopToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function loopToolCallFunc `json:"function"`
}

type loopToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// loopMessage is a chat message for the agent loop. Unlike chatMessage
// (single-shot, system+user only), it also carries an assistant message's
// tool_calls and a tool-result message's tool_call_id, so the full history
// can be replayed to the model each turn. Empty fields are omitted so a
// plain system/user/assistant/tool message serializes cleanly.
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
}

// maxQueryCompletionTokens caps narration length — a cost guard (the loop
// multiplies Groq calls; a runaway narration would multiply tokens too).
const maxQueryCompletionTokens = 1024

type loopResponse struct {
	Choices []struct {
		Message loopMessage `json:"message"`
	} `json:"choices"`
}

// chatCompletionLoop sends one Groq request with the given tool_choice and the
// full message history, and returns the assistant message — which carries
// either tool_calls (the loop must execute and feed back) or final content.
// toolChoice is "auto" for normal rounds and "none" on a forced-narration
// final call (Groq's documented way to make the model emit text instead of
// another tool round).
func (c *Client) chatCompletionLoop(ctx context.Context, model string, messages []loopMessage, tools []toolDef, toolChoice string) (loopMessage, error) {
	reqBody := loopRequest{
		Model:               model,
		Messages:            messages,
		Tools:               tools,
		ToolChoice:          toolChoice,
		Temperature:         0.1,
		MaxCompletionTokens: maxQueryCompletionTokens,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return loopMessage{}, fmt.Errorf("orchestrator: marshal loop request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return loopMessage{}, fmt.Errorf("orchestrator: build loop request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return loopMessage{}, fmt.Errorf("orchestrator: loop request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return loopMessage{}, fmt.Errorf("orchestrator: read loop response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return loopMessage{}, fmt.Errorf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body))
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
