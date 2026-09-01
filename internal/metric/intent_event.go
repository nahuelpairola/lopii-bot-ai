package metric

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"lopiibot.com/internal/database"
)

type int64Array []int64

func (a int64Array) Value() (driver.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	strs := make([]string, len(a))
	for i, v := range a {
		strs[i] = fmt.Sprintf("%d", v)
	}
	return "{" + strings.Join(strs, ",") + "}", nil
}

type IntentEvent struct {
	ID                uint64     `gorm:"primaryKey"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UserID            uint64     `gorm:"column:user_id;not null"`
	RawMessage        string     `gorm:"column:raw_message;not null"`
	Intent            string     `gorm:"column:intent;not null"`
	NeedsConfirmation bool       `gorm:"column:needs_confirmation;not null"`
	Outcome           string     `gorm:"column:outcome;not null"`
	ResolvedAt        *time.Time `gorm:"column:resolved_at"`
	WasCorrect        *bool      `gorm:"column:was_correct"`
	TraceID           string     `gorm:"column:trace_id"`
}

func (IntentEvent) TableName() string {
	return "intent_events"
}

type repository struct {
	db *database.Connection
}

func InitRepository(conn *database.Connection) *repository {
	return &repository{db: conn}
}

func (r *repository) Log(userID uint64, traceID, rawMessage, intent string, needsConfirmation bool, outcome string) error {
	r.db.DB.Model(&IntentEvent{}).
		Where("user_id = ? AND outcome = ?", userID, "pending").
		Updates(map[string]interface{}{"outcome": "abandoned", "resolved_at": time.Now()})
	return r.db.DB.Create(&IntentEvent{
		UserID:            userID,
		TraceID:           traceID,
		RawMessage:        rawMessage,
		Intent:            intent,
		NeedsConfirmation: needsConfirmation,
		Outcome:           outcome,
	}).Error
}

func (r *repository) SetIntentIfQueued(userID uint64, intent string) error {
	sub := r.db.DB.Model(&IntentEvent{}).
		Select("id").
		Where("user_id = ? AND outcome = ? AND intent = ?", userID, "pending", "QUEUED").
		Order("id DESC").
		Limit(1)
	return r.db.DB.Model(&IntentEvent{}).
		Where("id = (?)", sub).
		Update("intent", intent).Error
}

func (r *repository) Resolve(userID uint64, outcome string, movementIDs []uint) error {
	now := time.Now()
	sub := r.db.DB.Model(&IntentEvent{}).
		Select("id").
		Where("user_id = ? AND outcome = ?", userID, "pending").
		Order("id DESC").
		Limit(1)
	updates := map[string]interface{}{"outcome": outcome, "resolved_at": now}
	if len(movementIDs) > 0 {
		ids := make(int64Array, len(movementIDs))
		for i, id := range movementIDs {
			ids[i] = int64(id)
		}
		updates["movement_ids"] = ids
	}
	return r.db.DB.Model(&IntentEvent{}).
		Where("id = (?)", sub).
		Updates(updates).Error
}
