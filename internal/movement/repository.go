package movement

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
)

type movementType string

const (
	Expense  movementType = constants.Expense
	Income   movementType = constants.Income
	Transfer movementType = constants.Transfer
)

type Movement struct {
	gorm.Model
	TransactionID *uuid.UUID        `gorm:"column:transaction_id"`
	UserID        uint64            `gorm:"column:user_id;not null"`
	AccountID     *uint64           `gorm:"column:account_id"`
	SubcategoryID uint64            `gorm:"column:subcategory_id;not null"`
	Date          time.Time         `gorm:"column:date;not null"`
	Type          movementType      `gorm:"column:type;type:movement_type;not null"`
	Amount        decimal.Decimal   `gorm:"column:amount;type:numeric(15,2);not null"`
	Currency      currency.Currency `gorm:"column:currency;type:currency_type;not null"`
	PaymentMethod *string           `gorm:"column:payment_method"`
	Merchant      *string           `gorm:"column:merchant"`
	Description   *string           `gorm:"column:description"`
}

func (Movement) TableName() string {
	return "movements"
}

type repository struct {
	db *database.Connection
}

func InitRepository(conn *database.Connection) *repository {
	return &repository{db: conn}
}

// InsertBatch inserta todos los movements en una sola transacción: si
// alguno falla, se revierten los que ya se hayan insertado.
func (r *repository) InsertBatch(ms []Movement) error {
	return r.db.DB.Transaction(func(tx *gorm.DB) error {
		for i := range ms {
			if err := tx.Create(&ms[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
