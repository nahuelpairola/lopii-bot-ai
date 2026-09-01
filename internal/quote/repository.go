package quote

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"lopiibot.com/internal/database"
)

const insertBatchSize = 1000

type repository struct{ conn *database.Connection }

func NewRepository(conn *database.Connection) *repository { return &repository{conn: conn} }

func (r *repository) LatestQuoteDate() (*time.Time, error) {
	var dates []time.Time
	err := r.conn.DB.Model(&Quote{}).
		Order("date DESC").Limit(1).
		Pluck("date", &dates).Error
	if err != nil || len(dates) == 0 {
		return nil, err
	}
	return &dates[0], nil
}

func (r *repository) InsertQuotes(qs []Quote) error {
	if len(qs) == 0 {
		return nil
	}
	return r.conn.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "rate_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"bid", "ask"}),
	}).CreateInBatches(qs, insertBatchSize).Error
}

func (r *repository) InsertCPI(cs []CPI) error {
	if len(cs) == 0 {
		return nil
	}
	return r.conn.DB.Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(cs, insertBatchSize).Error
}

func (r *repository) FindRateOnOrBefore(date time.Time, rateType string) (*Quote, error) {
	var q Quote
	err := r.conn.DB.
		Where("rate_type = ? AND date <= ?", rateType, date.Format("2006-01-02")).
		Order("date DESC").
		Limit(1).
		Take(&q).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}
