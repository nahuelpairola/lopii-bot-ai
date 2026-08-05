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
	// TappedAt sella el PRIMER tap del botón del tip (nil = nunca lo tocó).
	// Se escribe una sola vez, así la métrica lee "usuarios que engancharon"
	// y no "cantidad de taps".
	TappedAt *time.Time `gorm:"column:tapped_at"`
}

func (UserNudge) TableName() string { return "user_nudges" }

type repository struct{ conn *database.Connection }

func NewRepository(conn *database.Connection) *repository { return &repository{conn: conn} }

// MarkSent records the nudge as sent. Idempotent (DoNothing on conflict).
func (r *repository) MarkSent(userID uint64, key string) error {
	return r.conn.DB.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&UserNudge{UserID: userID, NudgeKey: key, SentAt: time.Now()}).Error
}

// SentKeys returns every nudge key already sent to the user. One query
// replaces the per-nudge lookup: with a dozen nudges all sent, the old
// shape ran a dozen queries per message to end up sending nothing.
func (r *repository) SentKeys(userID uint64) ([]string, error) {
	var keys []string
	err := r.conn.DB.Model(&UserNudge{}).
		Where("user_id = ?", userID).
		Pluck("nudge_key", &keys).Error
	return keys, err
}

// MarkSentAgain is MarkSent for the one recurring nudge (the question menu):
// on conflict it UPDATES sent_at instead of ignoring the write. Without it,
// LastSentAt would stay frozen at the first send, the cooldown would read as
// elapsed forever, and the tip would fire on every single message.
func (r *repository) MarkSentAgain(userID uint64, key string) error {
	return r.conn.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "nudge_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"sent_at"}),
	}).Create(&UserNudge{UserID: userID, NudgeKey: key, SentAt: time.Now()}).Error
}

// MarkTapped seals the FIRST tap of a nudge's button. The tapped_at IS NULL
// predicate is what makes it set-once: a later tap leaves the timestamp alone.
func (r *repository) MarkTapped(userID uint64, key string) error {
	return r.conn.DB.Model(&UserNudge{}).
		Where("user_id = ? AND nudge_key = ? AND tapped_at IS NULL", userID, key).
		Update("tapped_at", time.Now()).Error
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
