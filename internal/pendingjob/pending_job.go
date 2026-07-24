package pendingjob

import "time"

// PendingJob es una fila de pending_llm_jobs: un mensaje del usuario cacheado
// tras un 429 terminal de Groq, para despacharlo cuando el cupo se libere.
// Payload es JSON opaco (la semántica por Kind la owna el paquete messaging).
// CreatedAt lo autopopula GORM (campo CreatedAt → autoCreateTime).
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
