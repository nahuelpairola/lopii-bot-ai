package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"lopiibot.com/internal/trace"
)

type Client struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string
	recorder   LLMRecorder
}

func NewClient(apiKey, baseURL string, timeout time.Duration, recorder LLMRecorder) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		apiKey:     apiKey,
		baseURL:    baseURL,
		recorder:   recorder,
	}
}

const (
	maxSendAttempts = 2
	baseBackoff     = 250 * time.Millisecond
	maxBackoff      = 20 * time.Second
)

var groqRetryAfterPattern = regexp.MustCompile(`(?i)try again in ([0-9smh.]+)`)

func parseBodyRetryAfter(body []byte) time.Duration {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0
	}
	m := groqRetryAfterPattern.FindStringSubmatch(parsed.Error.Message)
	if m == nil {
		return 0
	}
	d, err := time.ParseDuration(strings.TrimSuffix(m[1], "."))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

func (c *Client) send(ctx context.Context, callType, model string, payload []byte) ([]byte, error) {
	start := time.Now()
	var lastErr error
	var lastStatus int
	var lastRetryAfter time.Duration
	var lastHeader http.Header
	attempt := 0
	wait := baseBackoff
	for attempt = 0; attempt < maxSendAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				c.record(ctx, callType, model, start, attempt, lastStatus, ctx.Err().Error(), nil, nil)
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
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("orchestrator: read response: %w", err)
			continue
		}
		lastStatus = resp.StatusCode
		if resp.StatusCode == http.StatusOK {
			c.record(ctx, callType, model, start, attempt+1, resp.StatusCode, "", resp.Header, body)
			return body, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body))
			ra := retryAfter(resp.Header)
			if ra == 0 {
				if ra = parseResetTokens(resp.Header); ra == 0 {
					ra = parseBodyRetryAfter(body)
				}
			}
			lastRetryAfter = ra
			lastHeader = resp.Header
			attempt++
			break
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body))
			if ra := retryAfter(resp.Header); ra > 0 {
				wait = ra
				if wait > maxBackoff {
					wait = maxBackoff
				}
			}
			continue
		}
		if callType == callTypeCreate && isToolUseFailed(body) && attempt == 0 {
			lastErr = fmt.Errorf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body))
			continue
		}
		c.record(ctx, callType, model, start, attempt+1, resp.StatusCode, fmt.Sprintf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body)), resp.Header, nil)
		if isToolUseFailed(body) {
			return nil, fmt.Errorf("%w: %s", ErrNothingToExtract, string(body))
		}
		return nil, fmt.Errorf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body))
	}
	c.record(ctx, callType, model, start, attempt, lastStatus, errStr(lastErr), lastHeader, nil)
	if lastStatus == http.StatusTooManyRequests {
		return nil, &RateLimitedError{RetryAfter: lastRetryAfter, err: lastErr}
	}
	return nil, lastErr
}

func (c *Client) record(ctx context.Context, callType, model string, start time.Time, attempts, status int, errMsg string, header http.Header, body []byte) {
	if c.recorder == nil {
		return
	}
	rec := LLMCall{
		TraceID:    trace.ID(ctx),
		CallType:   callType,
		Model:      model,
		LatencyMs:  int(time.Since(start).Milliseconds()),
		HTTPStatus: status,
		Attempts:   attempts,
		Err:        errMsg,
	}
	if body != nil {
		rec.PromptTokens, rec.CompletionTokens, rec.TotalTokens = parseUsage(body)
		rec.ToolCalls = parseToolCalls(body)
	}
	if header != nil {
		rec.RateLimitRemainingRequests, rec.RateLimitRemainingTokens = parseRateLimitRemaining(header)
	}
	c.recorder.Record(rec)
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

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

type toolSchema struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

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

func (c *Client) chatCompletion(ctx context.Context, callType, model, systemPrompt, userMessage string, tool toolSchema) (json.RawMessage, error) {
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

	body, err := c.send(ctx, callType, model, payload)
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

var ErrNothingToExtract = errors.New("orchestrator: el modelo no encontró nada que extraer")

type RateLimitedError struct {
	RetryAfter time.Duration
	err        error
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("orchestrator: rate limited, retry after %s: %v", e.RetryAfter, e.err)
}
func (e *RateLimitedError) Unwrap() error { return e.err }

func isToolUseFailed(body []byte) bool {
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	return payload.Error.Code == "tool_use_failed"
}
