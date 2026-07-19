package nudge

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"lopiibot.com/internal/database"
)

// UserNudge marks that a contextual nudge (see messaging.nudgeDef) was sent
// to a user, once-ever per (user, key) — the primary key doubles as the
// dedup guard.
type UserNudge struct {
	UserID   uint64    `gorm:"primaryKey;column:user_id"`
	NudgeKey string    `gorm:"primaryKey;column:nudge_key"`
	SentAt   time.Time `gorm:"column:sent_at"`
}

func (UserNudge) TableName() string { return "user_nudges" }

type repository struct{ conn *database.Connection }

func NewRepository(conn *database.Connection) *repository { return &repository{conn: conn} }

// WasSent reports whether this nudge already fired for the user (once-ever).
func (r *repository) WasSent(userID uint64, key string) (bool, error) {
	var n UserNudge
	err := r.conn.DB.Where("user_id = ? AND nudge_key = ?", userID, key).First(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return err == nil, err
}

// MarkSent records the nudge as sent. Idempotent (DoNothing on conflict).
func (r *repository) MarkSent(userID uint64, key string) error {
	return r.conn.DB.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&UserNudge{UserID: userID, NudgeKey: key, SentAt: time.Now()}).Error
}

// LastSentAt returns when the user's most recent nudge fired, for the
// global cooldown — nil if none yet.
func (r *repository) LastSentAt(userID uint64) (*time.Time, error) {
	var n UserNudge
	err := r.conn.DB.Where("user_id = ?", userID).Order("sent_at DESC").First(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n.SentAt, nil
}
