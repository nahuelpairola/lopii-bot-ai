package movement

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
	"lopiibot.com/internal/subcategory"
)

type movementType string

const (
	Expense  movementType = constants.Expense
	Income   movementType = constants.Income
	Transfer movementType = constants.Transfer
)

type Movement struct {
	gorm.Model
	TransactionID *uuid.UUID                 `gorm:"column:transaction_id"`
	UserID        uint64                     `gorm:"column:user_id;not null"`
	AccountID     *uint64                    `gorm:"column:account_id"`
	SubcategoryID uint64                     `gorm:"column:subcategory_id;not null"`
	Subcategory   *subcategory.Subcategory   `gorm:"foreignKey:SubcategoryID"`
	Date          time.Time                  `gorm:"column:date;not null"`
	Type          movementType               `gorm:"column:type;type:movement_type;not null"`
	Amount        decimal.Decimal            `gorm:"column:amount;type:numeric(15,2);not null"`
	Currency      currency.Currency          `gorm:"column:currency;type:currency_type;not null"`
	PaymentMethod *string                    `gorm:"column:payment_method"`
	Merchant      *string                    `gorm:"column:merchant"`
	Description   *string                    `gorm:"column:description"`
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
			if err := tx.Omit("Subcategory").Create(&ms[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

var ErrMovementNotFound = errors.New("movement not found")

// FindSimilarForUser busca movimientos del usuario cuya description o
// merchant sean textualmente similares a query (vía pg_trgm), entre since
// y until (until nil = sin tope superior). Es el único mecanismo de
// resolución de referencias del bot: UPDATE/DELETE (ver
// reference_resolution.go) y el chequeo de duplicados de CREATE (ver
// startMovementCreate) lo usan por igual. Preload("Subcategory") evita
// que cada caller tenga que resolver subcategory_id → nombre a mano.
func (r *repository) FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]Movement, error) {
	var ms []Movement
	q := r.db.DB.Preload("Subcategory").
		Where("user_id = ? AND date >= ? AND (similarity(description, ?) > 0.2 OR similarity(merchant, ?) > 0.2)",
			userID, since, query, query)
	if until != nil {
		q = q.Where("date <= ?", *until)
	}
	err := q.Order("date DESC, id DESC").Find(&ms).Error
	return ms, err
}

// SoftDeleteByIDs borra (soft-delete vía deleted_at) todas las filas
// listadas en un solo UPDATE. Devuelve ErrMovementNotFound si ninguna
// coincide (0 filas afectadas).
func (r *repository) SoftDeleteByIDs(ids []uint) error {
	result := r.db.DB.Where("id IN ?", ids).Delete(&Movement{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrMovementNotFound
	}
	return nil
}

// ReplaceMovements implementa la regla de UPDATE (siempre DELETE+INSERT
// atómico, nunca patch parcial): borra las filas viejas (por ID, cubre
// tanto un movimiento suelto como un grupo entero) e inserta las nuevas
// dentro de una sola transacción de DB, para que una falla parcial no
// deje el grupo mitad borrado, mitad insertado.
func (r *repository) ReplaceMovements(oldIDs []uint, newMovements []Movement) error {
	return r.db.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id IN ?", oldIDs).Delete(&Movement{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrMovementNotFound
		}
		for i := range newMovements {
			if err := tx.Omit("Subcategory").Create(&newMovements[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SumAmountForAccount implementa la regla "el balance de una cuenta
// nunca se guarda, siempre se computa": suma el amount de todos los
// movimientos no borrados de esa cuenta. Usada por el cálculo de
// ganancia de rescate de FCI (ver movement_create_flow.go).
func (r *repository) SumAmountForAccount(accountID uint64) (decimal.Decimal, error) {
	var total decimal.NullDecimal
	err := r.db.DB.Model(&Movement{}).
		Where("account_id = ?", accountID).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&total).Error
	if err != nil {
		return decimal.Zero, err
	}
	return total.Decimal, nil
}
