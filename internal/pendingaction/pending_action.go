package pendingaction

import "time"

type OpenQuestion struct {
	Key     string   `json:"key"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options,omitempty"`
	Answer  string   `json:"answer,omitempty"`
}

type PendingAction struct {
	ID        uint64    `gorm:"primaryKey"`
	UserID    uint64    `gorm:"column:user_id;not null"`
	Tool      string    `gorm:"column:tool;not null"`
	Payload   []byte    `gorm:"column:payload;type:jsonb"`
	Questions []byte    `gorm:"column:questions;type:jsonb"`
	Budget    int       `gorm:"column:budget;not null"`
	Position  int       `gorm:"column:position;not null"`
	TraceID   string    `gorm:"column:trace_id"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (PendingAction) TableName() string {
	return "pending_actions"
}
