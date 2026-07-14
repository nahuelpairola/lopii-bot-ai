package metric

import "time"

// LLMCall es una fila de llm_calls: una llamada HTTP a Groq (grano send()).
// CreatedAt lo autopopula GORM. Punteros = nullable (no hubo body/headers en error).
type LLMCall struct {
	ID                         uint64    `gorm:"primaryKey"`
	CreatedAt                  time.Time `gorm:"column:created_at"`
	TraceID                    string    `gorm:"column:trace_id;not null"`
	CallType                   string    `gorm:"column:call_type;not null"`
	Model                      string    `gorm:"column:model;not null"`
	PromptTokens               int       `gorm:"column:prompt_tokens"`
	CompletionTokens           int       `gorm:"column:completion_tokens"`
	TotalTokens                int       `gorm:"column:total_tokens"`
	LatencyMs                  int       `gorm:"column:latency_ms;not null"`
	HTTPStatus                 int       `gorm:"column:http_status"`
	Attempts                   int       `gorm:"column:attempts"`
	Error                      string    `gorm:"column:error"`
	RateLimitRemainingRequests *int      `gorm:"column:ratelimit_remaining_requests"`
	RateLimitRemainingTokens   *int      `gorm:"column:ratelimit_remaining_tokens"`
}

func (LLMCall) TableName() string { return "llm_calls" }

// RequestTrace es una fila de request_traces: el spine de un update de Telegram.
// UserID nullable: un update de usuario desconocido / pre-auth no lo tiene.
type RequestTrace struct {
	ID         uint64    `gorm:"primaryKey"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	TraceID    string    `gorm:"column:trace_id;not null"`
	UserID     *uint64   `gorm:"column:user_id"`
	UpdateType string    `gorm:"column:update_type;not null"`
	ReceivedAt time.Time `gorm:"column:received_at;not null"`
	LatencyMs  int       `gorm:"column:latency_ms;not null"`
	Error      string    `gorm:"column:error"`
}

func (RequestTrace) TableName() string { return "request_traces" }
