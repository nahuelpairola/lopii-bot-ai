package quote

import (
	"time"

	"github.com/shopspring/decimal"
)

type Quote struct {
	Date     time.Time       `gorm:"primaryKey;column:date"`
	RateType string          `gorm:"primaryKey;column:rate_type"`
	Bid      decimal.Decimal `gorm:"column:bid"`
	Ask      decimal.Decimal `gorm:"column:ask"`
}

func (Quote) TableName() string { return "usd_quotes" }

type CPI struct {
	Month time.Time       `gorm:"primaryKey;column:month"`
	Value decimal.Decimal `gorm:"column:value"`
}

func (CPI) TableName() string { return "monthly_cpi" }
