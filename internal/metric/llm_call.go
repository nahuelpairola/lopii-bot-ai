package metric

import "time"

func (r *repository) InsertLLMCall(c *LLMCall) error {
	return r.db.DB.Create(c).Error
}

func (r *repository) InsertRequestTrace(traceID string, userID *uint64, updateType string, receivedAt time.Time, latencyMs int, errMsg string) error {
	return r.db.DB.Create(&RequestTrace{
		TraceID:    traceID,
		UserID:     userID,
		UpdateType: updateType,
		ReceivedAt: receivedAt,
		LatencyMs:  latencyMs,
		Error:      errMsg,
	}).Error
}

func (r *repository) DeleteOlderThan(cutoff time.Time) error {
	if err := r.db.DB.Where("created_at < ?", cutoff).Delete(&LLMCall{}).Error; err != nil {
		return err
	}
	if err := r.db.DB.Where("created_at < ?", cutoff).Delete(&RequestTrace{}).Error; err != nil {
		return err
	}
	return r.db.DB.Where("created_at < ?", cutoff).Delete(&IntentEvent{}).Error
}

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
	ToolCalls                  *string   `gorm:"column:tool_calls;type:jsonb"`
}

func (LLMCall) TableName() string { return "llm_calls" }

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
