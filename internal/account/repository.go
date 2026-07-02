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
	Type      accountType       `gorm:"column:type"`
	Currency  currency.Currency `gorm:"column:currency"`
	IsDefault bool              `gorm:"column:is_default"`
}

var ErrAccountAlreadyExists = errors.New("an account with that name and currency already exists")

type accountType string

const (
	StandardType accountType = "standard"
	SystemType   accountType = "system"
)

func (c accountType) String() string {
	return string(c)
}

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

// HasDefaultForCurrency indica si el usuario ya tiene una cuenta default
// en la moneda dada.
func (r *repository) HasDefaultForCurrency(userID uint64, currency currency.Currency) bool {
	_, err := r.FindDefaultByCurrency(userID, currency)
	return err == nil
}

// CountByUserID cuenta cuántas cuentas tiene el usuario en total.
func (r *repository) CountByUserID(userID uint64) (int64, error) {
	var count int64
	err := r.conn.DB.Model(&Account{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

// UnsetDefault saca el flag default de la cuenta que hoy lo tiene en esa
// moneda, dejando lugar para que otra pase a ser la nueva default.
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

// GetForUser devuelve todas las cuentas del usuario (alias para FindByUserID).
func (r *repository) GetForUser(userID uint64) ([]Account, error) {
	return r.FindByUserID(userID)
}

// GetAccount obtiene una cuenta específica por ID.
func (r *repository) GetAccount(id uint64) (*Account, error) {
	var a Account
	err := r.conn.DB.First(&a, id).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// SetDefault marca una cuenta como default, desmarcando las anteriores de esa moneda.
func (r *repository) SetDefault(accountID uint64) error {
	return r.conn.DB.Model(&Account{}).
		Where("id = ?", accountID).
		Update("is_default", true).Error
}

// isUniqueViolation detecta el código de error de Postgres para
// violación de constraint único (23505), sin acoplar el resto del
// código a pgconn directamente.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
