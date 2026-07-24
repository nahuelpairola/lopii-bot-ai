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

// Client is a minimal Groq chat-completions client using tool calling
// (Groq's API is OpenAI-compatible, so no SDK dependency is needed).
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
	// maxSendAttempts caps total tries (1 original + 3 retries) on a transient
	// Groq failure. Retry fires ONLY on failure, so the happy path adds 0ms.
	maxSendAttempts = 4
	// baseBackoff is the first retry wait for the blind exponential path (no
	// Retry-After header, no parseable body wait — plain 5xx/network errors);
	// it doubles each attempt (250ms, 500ms, 1s).
	baseBackoff = 250 * time.Millisecond
	// maxBackoff caps any single wait, including an honored Retry-After header
	// or a body-parsed Groq TPM wait (see parseGroqRetryAfterSeconds) — a slow
	// 429 never freezes the user longer than this. Real Groq TPM waits
	// observed up to ~15s, so this must be well above the old 1s cap to be
	// honored at all instead of silently truncated to uselessness.
	maxBackoff = 20 * time.Second
)

// groqRetryAfterPattern captura el token de duración completo del 429 de Groq,
// e.g. "try again in 4.185s" o "try again in 16m29.28s" (TPD, min+seg).
var groqRetryAfterPattern = regexp.MustCompile(`(?i)try again in ([0-9smh.]+)`)

// parseBodyRetryAfter extrae el wait que recomienda el body del 429 de Groq.
// Usa time.ParseDuration (maneja "16m29.28s" nativo). 0 si no matchea o no
// parsea — un body inesperado nunca panichea ni bloquea el retry.
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
	// El charset de la regex incluye "." para admitir fracciones ("29.28s"), lo
	// que también atrapa el punto final de la oración cuando el mensaje termina
	// justo ahí ("...16m29.28s."). ParseDuration no tolera ese sufijo.
	d, err := time.ParseDuration(strings.TrimSuffix(m[1], "."))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// send POSTs payload to Groq's chat/completions and returns the 200 body. It
// retries only transient failures — a network error, HTTP 429, or any 5xx —
// up to maxSendAttempts. Wait between retries prefers, in order: the 429's
// Retry-After header; Groq's body-stated TPM wait (parseGroqRetryAfterSeconds
// — Groq doesn't send a header for token-based 429s); else blind exponential
// backoff. All capped at maxBackoff. A non-429 4xx (a malformed request = our
// bug) fails immediately. The whole sequence is bound by ctx. This is the
// single Groq I/O chokepoint: every call type (router, create, update,
// delete, onboarding, query) inherits the retry.
func (c *Client) send(ctx context.Context, callType, model string, payload []byte) ([]byte, error) {
	start := time.Now()
	var lastErr error
	var lastStatus int
	var lastRetryAfter time.Duration
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
			continue // network error — transient, retry
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
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body))
			// Prioridad del wait: header Retry-After (req-count) > header reset-tokens
			// (TPM/TPD) > body "try again in <dur>". Groq no manda Retry-After para
			// límites por token, de ahí los fallbacks.
			ra := retryAfter(resp.Header)
			if ra == 0 && resp.StatusCode == http.StatusTooManyRequests {
				if ra = parseResetTokens(resp.Header); ra == 0 {
					ra = parseBodyRetryAfter(body)
				}
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				lastRetryAfter = ra // el wait crudo de Groq, sin capear (la cola lo usa)
			}
			if ra > 0 {
				wait = ra
				if wait > maxBackoff {
					wait = maxBackoff
				}
			}
			continue // transient — retry
		}
		// non-retryable (a non-429 4xx = malformed request, our bug)
		c.record(ctx, callType, model, start, attempt+1, resp.StatusCode, fmt.Sprintf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body)), resp.Header, nil)
		if isToolUseFailed(body) {
			return nil, fmt.Errorf("%w: %s", ErrNothingToExtract, string(body))
		}
		return nil, fmt.Errorf("orchestrator: groq returned status %d: %s", resp.StatusCode, string(body))
	}
	c.record(ctx, callType, model, start, attempt, lastStatus, errStr(lastErr), nil, nil)
	if lastStatus == http.StatusTooManyRequests {
		return nil, &RateLimitedError{RetryAfter: lastRetryAfter, err: lastErr}
	}
	return nil, lastErr
}

// record arma el LLMCall y lo emite fire-and-forget (nil-safe).
// ponytail: record síncrono adentro de send; si el insert agrega latencia
// medible, moverlo a un channel buffered. A ~4 usuarios no hace falta.
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

// ErrNothingToExtract señala que el modelo se negó a llamar a la tool porque el
// mensaje no traía lo que la tool necesita — "quiero crear una categoría" sin
// decir cuál, o "ponelo ahí" sin monto.
//
// No es un error de infraestructura ni un bug nuestro: el modelo hizo lo
// correcto al no inventar. Groq lo devuelve como HTTP 400 con code
// "tool_use_failed" porque mandamos tool_choice=required, así que hay que
// distinguirlo del 400 genuino (request malformado) para poder pedirle al
// usuario el dato que falta en vez de mostrarle "algo salió mal".
var ErrNothingToExtract = errors.New("orchestrator: el modelo no encontró nada que extraer")

// RateLimitedError es el fallo terminal de send cuando el último intento fue un
// 429: envuelve el wait recomendado por Groq para que la cola de pending jobs
// (internal/pendingjob) sepa cuándo reintentar. Sube por errors.As por los 9
// callers sin cambiar ninguna firma.
type RateLimitedError struct {
	RetryAfter time.Duration
	err        error
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("orchestrator: rate limited, retry after %s: %v", e.RetryAfter, e.err)
}
func (e *RateLimitedError) Unwrap() error { return e.err }

// isToolUseFailed reconoce el 400 de Groq que en realidad significa "no había
// nada que extraer". Se parsea el código en vez de buscar la subcadena para no
// confundirlo con un mensaje de error que apenas lo mencione.
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
