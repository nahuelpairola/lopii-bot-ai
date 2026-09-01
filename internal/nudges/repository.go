package nudges

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"lopiibot.com/internal/database"
)

type UserNudge struct {
	UserID   uint64     `gorm:"primaryKey;column:user_id"`
	NudgeKey string     `gorm:"primaryKey;column:nudge_key"`
	SentAt   time.Time  `gorm:"column:sent_at"`
	TappedAt *time.Time `gorm:"column:tapped_at"`
}

func (UserNudge) TableName() string { return "user_nudges" }

type repository struct{ conn *database.Connection }

func NewRepository(conn *database.Connection) *repository { return &repository{conn: conn} }

func (r *repository) MarkSent(userID uint64, key string) error {
	return r.conn.DB.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&UserNudge{UserID: userID, NudgeKey: key, SentAt: time.Now()}).Error
}

func (r *repository) SentKeys(userID uint64) ([]string, error) {
	var keys []string
	err := r.conn.DB.Model(&UserNudge{}).
		Where("user_id = ?", userID).
		Pluck("nudge_key", &keys).Error
	return keys, err
}

func (r *repository) MarkSentAgain(userID uint64, key string) error {
	return r.conn.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "nudge_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"sent_at"}),
	}).Create(&UserNudge{UserID: userID, NudgeKey: key, SentAt: time.Now()}).Error
}

func (r *repository) MarkTapped(userID uint64, key string) error {
	return r.conn.DB.Model(&UserNudge{}).
		Where("user_id = ? AND nudge_key = ? AND tapped_at IS NULL", userID, key).
		Update("tapped_at", time.Now()).Error
}

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
