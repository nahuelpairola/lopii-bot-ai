package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a minimal Groq chat-completions client using tool calling
// (Groq's API is OpenAI-compatible, so no SDK dependency is needed).
type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
}

func NewClient(apiKey, baseURL string, timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		apiKey:     apiKey,
		baseURL:    baseURL,
	}
}

// toolSchema describes the single tool a call forces the model to
// invoke via tool_choice, so the response is always structured JSON,
// never free text that needs to be scraped out of a reasoning preamble.
type toolSchema struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

// flexBool decodes a JSON boolean OR a JSON string "true"/"false". Groq's
// tool-calling models occasionally emit a stringified boolean for a field
// declared boolean in the schema; Groq validates arguments against the
// schema server-side and 400s before this code ever sees the payload, so
// the schema itself must declare the field as ["boolean","string"] for
// this leniency to matter — see routerTool/updateTool/deleteTool.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	var v bool
	if err := json.Unmarshal(data, &v); err == nil {
		*b = flexBool(v)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("flexBool: %s is neither a boolean nor a string", data)
	}
	switch s {
	case "true":
		*b = true
	case "false":
		*b = false
	default:
		return fmt.Errorf("flexBool: unrecognized string value %q", s)
	}
	return nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type toolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type toolDef struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

type toolChoiceFunction struct {
	Name string `json:"name"`
}

type toolChoiceForce struct {
	Type     string             `json:"type"`
	Function toolChoiceFunction `json:"function"`
}

type chatCompletionRequest struct {
	Model       string          `json:"model"`
	Messages    []chatMessage   `json:"messages"`
	Tools       []toolDef       `json:"tools"`
	ToolChoice  toolChoiceForce `json:"tool_choice"`
	Temperature float64         `json:"temperature"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			ToolCalls []struct {
				Function struct {
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

// chatCompletion sends one Groq tool-calling request, forcing the model
// to call tool, and returns the raw JSON arguments it produced.
func (c *Client) chatCompletion(ctx context.Context, model, systemPrompt, userMessage string, tool toolSchema) (json.RawMessage, error) {
	reqBody := chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		Tools: []toolDef{{
			Type: "function",
			Function: toolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
			},
		}},
		ToolChoice: toolChoiceForce{
			Type:     "function",
			Function: toolChoiceFunction{Name: tool.Name},
		},
		Temperature: 0.1,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("orchestrator: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("orchestrator: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("orchestrator: unmarshal response: %w", err)
	}
	if len(parsed.Choices) == 0 || len(parsed.Choices[0].Message.ToolCalls) == 0 {
		return nil, fmt.Errorf("orchestrator: groq response had no tool call")
	}

	return json.RawMessage(parsed.Choices[0].Message.ToolCalls[0].Function.Arguments), nil
}
