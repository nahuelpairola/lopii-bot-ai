package reminder

import (
	"time"

	"gorm.io/gorm/clause"
	"lopiibot.com/internal/database"
)

type Reminder struct {
	UserID               uint64     `gorm:"primaryKey;column:user_id"`
	WindowStartMin       int        `gorm:"column:window_start_min"`
	WindowEndMin         int        `gorm:"column:window_end_min"`
	Enabled              bool       `gorm:"column:enabled"`
	LastRemindedOn       *time.Time `gorm:"column:last_reminded_on"`
	WeeklySummaryEnabled bool       `gorm:"column:weekly_summary_enabled"`
	LastSummaryOn        *time.Time `gorm:"column:last_summary_on"`
	LastMonthlySummaryOn *time.Time `gorm:"column:last_monthly_summary_on"`
	CreatedAt            time.Time  `gorm:"column:created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at"`
}

func (r Reminder) MidpointMin() int { return (r.WindowStartMin + r.WindowEndMin) / 2 }

type repository struct {
	conn *database.Connection
}

func NewRepository(conn *database.Connection) *repository {
	return &repository{conn: conn}
}

func (r *repository) Upsert(rem *Reminder) error {
	return r.conn.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"window_start_min", "window_end_min", "enabled", "weekly_summary_enabled", "updated_at"}),
	}).Create(rem).Error
}

func (r *repository) Disable(userID uint64) error {
	return r.conn.DB.Model(&Reminder{}).Where("user_id = ?", userID).Update("enabled", false).Error
}

func (r *repository) FindByUserID(userID uint64) (*Reminder, error) {
	var rem Reminder
	if err := r.conn.DB.Where("user_id = ?", userID).First(&rem).Error; err != nil {
		return nil, err
	}
	return &rem, nil
}

func (r *repository) ListDue(before time.Time) ([]Reminder, error) {
	var rs []Reminder
	err := r.conn.DB.
		Where("enabled AND (last_reminded_on IS NULL OR last_reminded_on < ?)", before).
		Find(&rs).Error
	return rs, err
}

func (r *repository) SetLastRemindedOn(userID uint64, date time.Time) error {
	return r.conn.DB.Model(&Reminder{}).Where("user_id = ?", userID).Update("last_reminded_on", date).Error
}

func (r *repository) ListWeeklyDue(before time.Time) ([]Reminder, error) {
	var rs []Reminder
	err := r.conn.DB.
		Where("weekly_summary_enabled AND (last_summary_on IS NULL OR last_summary_on < ?)", before).
		Find(&rs).Error
	return rs, err
}

func (r *repository) SetLastSummaryOn(userID uint64, date time.Time) error {
	return r.conn.DB.Model(&Reminder{}).Where("user_id = ?", userID).Update("last_summary_on", date).Error
}

func (r *repository) ListMonthlyDue(before time.Time) ([]uint64, error) {
	var ids []uint64
	err := r.conn.DB.
		Table("users u").
		Joins("LEFT JOIN reminders r ON r.user_id = u.id").
		Where("r.last_monthly_summary_on IS NULL OR r.last_monthly_summary_on < ?", before).
		Order("u.id").
		Pluck("u.id", &ids).Error
	return ids, err
}

func (r *repository) SetLastMonthlySummaryOn(userID uint64, date time.Time) error {
	return r.conn.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_monthly_summary_on"}),
	}).Create(&Reminder{
		UserID:               userID,
		Enabled:              false,
		WeeklySummaryEnabled: false,
		LastMonthlySummaryOn: &date,
	}).Error
}

func (r *repository) SetWeeklySummary(userID uint64, enabled bool) error {
	return r.conn.DB.Model(&Reminder{}).Where("user_id = ?", userID).Update("weekly_summary_enabled", enabled).Error
}
