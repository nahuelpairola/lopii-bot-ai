package account

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/database"
)

type Account struct {
	gorm.Model
	UserID    uint64            `gorm:"column:user_id"`
	Name      string            `gorm:"column:name"`
	Currency  currency.Currency `gorm:"column:currency"`
	IsDefault bool              `gorm:"column:is_default"`
}

var ErrAccountAlreadyExists = errors.New("an account with that name and currency already exists")

type repository struct {
	conn *database.Connection
}

func NewRepository(conn *database.Connection) *repository {
	return &repository{conn: conn}
}

func (r *repository) FindByUserID(userID uint64) ([]Account, error) {
	var accounts []Account
	err := r.conn.DB.Where("user_id = ?", userID).Find(&accounts).Error
	return accounts, err
}

func (r *repository) FindDefaultByCurrency(userID uint64, currency currency.Currency) (*Account, error) {
	var a Account
	err := r.conn.DB.
		Where("user_id = ? AND currency = ? AND is_default = TRUE", userID, currency.String()).
		First(&a).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *repository) HasDefaultForCurrency(userID uint64, currency currency.Currency) bool {
	_, err := r.FindDefaultByCurrency(userID, currency)
	return err == nil
}

func (r *repository) UnsetDefault(userID uint64, currency currency.Currency) error {
	return r.conn.DB.Model(&Account{}).
		Where("user_id = ? AND currency = ? AND is_default = TRUE", userID, currency).
		Update("is_default", false).Error
}

func (r *repository) Insert(a *Account) error {
	err := r.conn.DB.Create(a).Error
	if isUniqueViolation(err) {
		return ErrAccountAlreadyExists
	}
	return err
}

func (r *repository) SoftDeleteByUserID(userID uint64) error {
	return r.conn.DB.Where("user_id = ?", userID).Delete(&Account{}).Error
}

func (r *repository) GetAccount(id uint64) (*Account, error) {
	var a Account
	err := r.conn.DB.First(&a, id).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *repository) SetDefault(accountID uint64) error {
	return r.conn.DB.Model(&Account{}).
		Where("id = ?", accountID).
		Update("is_default", true).Error
}

func (r *repository) Rename(accountID uint64, name string) error {
	err := r.conn.DB.Model(&Account{}).
		Where("id = ?", accountID).
		Update("name", name).Error
	if isUniqueViolation(err) {
		return ErrAccountAlreadyExists
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
