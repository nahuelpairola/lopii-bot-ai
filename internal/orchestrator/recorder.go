package orchestrator

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func parseUsage(body []byte) (prompt, completion, total int) {
	var u struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(body, &u)
	return u.Usage.PromptTokens, u.Usage.CompletionTokens, u.Usage.TotalTokens
}

func parseToolCalls(body []byte) string {
	var r struct {
		Choices []struct {
			Message struct {
				ToolCalls json.RawMessage `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &r); err != nil || len(r.Choices) == 0 {
		return ""
	}
	raw := r.Choices[0].Message.ToolCalls
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return ""
	}
	return string(raw)
}

func parseRateLimitRemaining(h http.Header) (*int, *int) {
	return atoiPtr(h.Get("x-ratelimit-remaining-requests")),
		atoiPtr(h.Get("x-ratelimit-remaining-tokens"))
}

type LLMCall struct {
	TraceID                    string
	CallType                   string
	Model                      string
	PromptTokens               int
	CompletionTokens           int
	TotalTokens                int
	LatencyMs                  int
	HTTPStatus                 int
	Attempts                   int
	Err                        string
	RateLimitRemainingRequests *int
	RateLimitRemainingTokens   *int
	ToolCalls                  string
}

type LLMRecorder interface {
	Record(LLMCall)
}

const (
	callTypeRouter         = "router"
	callTypeCreate         = "create"
	callTypeUpdate         = "update"
	callTypeDelete         = "delete"
	callTypeOnboarding     = "onboarding"
	callTypeQuery          = "query"
	callTypeCategoryCreate = "category_create"
	callTypeClassifier     = "classifier"
	callTypeAccountManage  = "account_manage"
	callTypeAgent          = "agent"
)

func parseResetTokens(h http.Header) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(h.Get("x-ratelimit-reset-tokens")))
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

func atoiPtr(s string) *int {
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &n
}
