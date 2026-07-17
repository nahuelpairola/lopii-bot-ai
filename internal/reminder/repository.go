package reminder

import (
	"time"

	"gorm.io/gorm/clause"
	"lopiibot.com/internal/database"
)

// Reminder is a user's single daily expense-logging reminder. Window bounds
// are minutes since ART midnight (e.g. 20:00 = 1200); the fire target is the
// midpoint, so sub-hour precision matters (20:00–21:00 -> 20:30). One row per
// user (PK user_id). Delete == disable (Enabled=false); no soft delete.
type Reminder struct {
	UserID               uint64     `gorm:"primaryKey;column:user_id"`
	WindowStartMin       int        `gorm:"column:window_start_min"`
	WindowEndMin         int        `gorm:"column:window_end_min"`
	Enabled              bool       `gorm:"column:enabled"`
	LastRemindedOn       *time.Time `gorm:"column:last_reminded_on"`
	WeeklySummaryEnabled bool       `gorm:"column:weekly_summary_enabled"`
	LastSummaryOn        *time.Time `gorm:"column:last_summary_on"`
	CreatedAt            time.Time  `gorm:"column:created_at"`
	UpdatedAt            time.Time  `gorm:"column:updated_at"`
}

// MidpointMin is the fire target: the middle of the window, in minutes since
// midnight. Derived, never stored.
func (r Reminder) MidpointMin() int { return (r.WindowStartMin + r.WindowEndMin) / 2 }

type repository struct {
	conn *database.Connection
}

func NewRepository(conn *database.Connection) *repository {
	return &repository{conn: conn}
}

// Upsert sets the user's reminder window and enables it, creating the row or
// overwriting the window on an existing one. last_reminded_on is left intact.
func (r *repository) Upsert(rem *Reminder) error {
	return r.conn.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"window_start_min", "window_end_min", "enabled", "weekly_summary_enabled", "updated_at"}),
	}).Create(rem).Error
}

// Disable turns the reminder off. No-op (no error) if the user has no row.
func (r *repository) Disable(userID uint64) error {
	return r.conn.DB.Model(&Reminder{}).Where("user_id = ?", userID).Update("enabled", false).Error
}

// FindByUserID returns the user's reminder, or gorm.ErrRecordNotFound if none.
func (r *repository) FindByUserID(userID uint64) (*Reminder, error) {
	var rem Reminder
	if err := r.conn.DB.Where("user_id = ?", userID).First(&rem).Error; err != nil {
		return nil, err
	}
	return &rem, nil
}

// ListDue returns enabled reminders not yet reminded on/after `before`
// (pass ART start-of-today). The sweeper further filters by midpoint + activity.
func (r *repository) ListDue(before time.Time) ([]Reminder, error) {
	var rs []Reminder
	err := r.conn.DB.
		Where("enabled AND (last_reminded_on IS NULL OR last_reminded_on < ?)", before).
		Find(&rs).Error
	return rs, err
}

// SetLastRemindedOn marks the reminder as fired for `date` (ART start-of-today).
func (r *repository) SetLastRemindedOn(userID uint64, date time.Time) error {
	return r.conn.DB.Model(&Reminder{}).Where("user_id = ?", userID).Update("last_reminded_on", date).Error
}

// ListWeeklyDue returns reminders opted into the weekly summary whose summary
// hasn't been sent on/after `before` (pass ART this-Monday start-of-day). The
// sweeper further gates by the fire hour.
func (r *repository) ListWeeklyDue(before time.Time) ([]Reminder, error) {
	var rs []Reminder
	err := r.conn.DB.
		Where("weekly_summary_enabled AND (last_summary_on IS NULL OR last_summary_on < ?)", before).
		Find(&rs).Error
	return rs, err
}

// SetLastSummaryOn marks the weekly summary as sent for `date` (ART this-Monday).
func (r *repository) SetLastSummaryOn(userID uint64, date time.Time) error {
	return r.conn.DB.Model(&Reminder{}).Where("user_id = ?", userID).Update("last_summary_on", date).Error
}

// SetWeeklySummary toggles ONLY the weekly-summary flag, never the daily window
// or `enabled`. No-op (no error) if the user has no row — the summary lives on
// the reminders row.
func (r *repository) SetWeeklySummary(userID uint64, enabled bool) error {
	return r.conn.DB.Model(&Reminder{}).Where("user_id = ?", userID).Update("weekly_summary_enabled", enabled).Error
}
