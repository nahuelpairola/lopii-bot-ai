package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
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

const (
	// maxSendAttempts caps total tries (1 original + 2 retries) on a transient
	// Groq failure. Retry fires ONLY on failure, so the happy path adds 0ms.
	maxSendAttempts = 3
	// baseBackoff is the first retry wait; it doubles each attempt (250ms, 500ms).
	baseBackoff = 250 * time.Millisecond
	// maxBackoff caps any single wait (including an honored Retry-After) so a
	// slow 429 never freezes the user longer than this.
	maxBackoff = 1 * time.Second
)

// send POSTs payload to Groq's chat/completions and returns the 200 body. It
// retries only transient failures — a network error, HTTP 429, or any 5xx —
// up to maxSendAttempts with short exponential backoff (honoring a 429's
// Retry-After header, capped at maxBackoff). A non-429 4xx (a malformed
// request = our bug) fails immediately. The whole sequence is bound by ctx.
// This is the single Groq I/O chokepoint: every call type (router, create,
// update, delete, onboarding, query) inherits the retry.
func (c *Client) send(ctx context.Context, payload []byte) ([]byte, error) {
	var lastErr error
	wait := baseBackoff
	for attempt := 0; attempt < maxSendAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			wait *= 2
			if wait > maxBackoff {
				wait = maxBackoff
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("orchestrator: build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("orchestrator: request failed: %w", err)
			continue // network error — transient, retry
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("orchestrator: read response: %w", err)
			continue
		}
		if resp.StatusCode == http.StatusOK {
			return body, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body))
			if ra := retryAfter(resp.Header); ra > 0 {
				wait = ra
				if wait > maxBackoff {
					wait = maxBackoff
				}
			}
			continue // transient — retry
		}
		// non-retryable (a non-429 4xx = malformed request, our bug)
		return nil, fmt.Errorf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil, lastErr
}

// retryAfter parses a Retry-After header expressed in whole seconds (Groq's
// form). A missing / HTTP-date / garbage value returns 0 → the caller keeps
// its exponential backoff.
// ponytail: seconds only; add HTTP-date parsing if Groq ever sends that form.
func retryAfter(h http.Header) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
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

	body, err := c.send(ctx, payload)
	if err != nil {
		return nil, err
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
