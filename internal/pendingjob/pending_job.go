package pendingjob

import "time"

type PendingJob struct {
	ID        uint64    `gorm:"primaryKey"`
	UserID    uint64    `gorm:"column:user_id;not null"`
	Kind      string    `gorm:"column:kind;not null"`
	Payload   []byte    `gorm:"column:payload;type:jsonb"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (PendingJob) TableName() string {
	return "pending_llm_jobs"
}
