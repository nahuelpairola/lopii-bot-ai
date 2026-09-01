package movement

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"lopiibot.com/internal/account"
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
	TransactionID *uuid.UUID               `gorm:"column:transaction_id"`
	UserID        uint64                   `gorm:"column:user_id;not null"`
	AccountID     *uint64                  `gorm:"column:account_id"`
	Account       *account.Account         `gorm:"foreignKey:AccountID"`
	SubcategoryID uint64                   `gorm:"column:subcategory_id;not null"`
	Subcategory   *subcategory.Subcategory `gorm:"foreignKey:SubcategoryID"`
	Date          time.Time                `gorm:"column:date;not null"`
	Type          movementType             `gorm:"column:type;type:movement_type;not null"`
	Amount        decimal.Decimal          `gorm:"column:amount;type:numeric(15,2);not null"`
	Currency      currency.Currency        `gorm:"column:currency;type:currency_type;not null"`
	PaymentMethod *string                  `gorm:"column:payment_method"`
	Description   *string                  `gorm:"column:description"`
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

type AccountOpening struct {
	Account  *account.Account
	Movement Movement
}

func (r *repository) InsertAccountsWithOpenings(items []AccountOpening) error {
	return r.db.DB.Transaction(func(tx *gorm.DB) error {
		for i := range items {
			if err := tx.Create(items[i].Account).Error; err != nil {
				return err
			}
			id := uint64(items[i].Account.ID)
			items[i].Movement.AccountID = &id
			if err := tx.Omit("Subcategory").Create(&items[i].Movement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

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

var ErrNoMovementIDs = errors.New("movement: no ids to delete")

func (r *repository) FindSimilarForUser(userID uint64, query string, since time.Time, until *time.Time) ([]Movement, error) {
	var ms []Movement
	q := r.db.DB.Preload("Subcategory").Preload("Account").
		Where("user_id = ? AND date >= ?", userID, since.Format("2006-01-02"))
	if until != nil {
		q = q.Where("date <= ?", until.Format("2006-01-02"))
	}
	err := q.Order("date DESC, id DESC").Find(&ms).Error
	return ms, err
}

func (r *repository) FindRecentlyCreatedForUser(userID uint64, since time.Time, limit int) ([]Movement, error) {
	var ms []Movement
	q := r.db.DB.Preload("Subcategory").Preload("Account").
		Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at DESC, id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&ms).Error
	return ms, err
}

func (r *repository) CountForUser(userID uint64) (int64, error) {
	var n int64
	err := r.db.DB.Model(&Movement{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}

func (r *repository) CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error) {
	var n int64
	err := r.db.DB.Model(&Movement{}).
		Where("user_id = ? AND subcategory_id = ?", userID, subcategoryID).
		Count(&n).Error
	return n, err
}

func (r *repository) ReassignSubcategory(userID uint64, fromID uint64, toID uint64) error {
	return r.db.DB.Unscoped().Model(&Movement{}).
		Where("user_id = ? AND subcategory_id = ?", userID, fromID).
		Update("subcategory_id", toID).Error
}

func (r *repository) TopDescriptionsBySubcategory(userID uint64, subcategoryID uint64, limit int) ([]string, error) {
	var descriptions []string
	err := r.db.DB.Model(&Movement{}).
		Where("user_id = ? AND subcategory_id = ?", userID, subcategoryID).
		Where("description IS NOT NULL AND description <> ''").
		Group("description").
		Order("COUNT(*) DESC").
		Limit(limit).
		Pluck("description", &descriptions).Error
	return descriptions, err
}

func (r *repository) SoftDeleteByIDs(ids []uint) error {
	if len(ids) == 0 {
		return ErrNoMovementIDs
	}
	var existing int64
	if err := r.db.DB.Unscoped().Model(&Movement{}).Where("id IN ?", ids).Count(&existing).Error; err != nil {
		return err
	}
	if existing == 0 {
		return ErrMovementNotFound
	}
	return r.db.DB.Where("id IN ?", ids).Delete(&Movement{}).Error
}

func (r *repository) SoftDeleteByUserID(userID uint64) error {
	return r.db.DB.Where("user_id = ?", userID).Delete(&Movement{}).Error
}

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

type MonthlyDelta struct {
	Month string          `gorm:"column:month"`
	Delta decimal.Decimal `gorm:"column:delta"`
}

func (r *repository) MonthlyDeltasForAccount(accountID uint64) ([]MonthlyDelta, error) {
	var rows []MonthlyDelta
	err := r.db.DB.Model(&Movement{}).
		Select("to_char(date, 'YYYY-MM') AS month, COALESCE(SUM(amount), 0) AS delta").
		Where("account_id = ?", accountID).
		Group("month").
		Order("month ASC").
		Scan(&rows).Error
	return rows, err
}

func (r *repository) ReassignAccount(fromID, toID uint64) error {
	return r.db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			UPDATE movements SET deleted_at = NOW()
			WHERE deleted_at IS NULL AND transaction_id IN (
				SELECT transaction_id FROM movements
				WHERE deleted_at IS NULL AND transaction_id IS NOT NULL
				  AND account_id IN (?, ?)
				GROUP BY transaction_id
				HAVING BOOL_OR(account_id = ?) AND BOOL_OR(account_id = ?)
			)`, fromID, toID, fromID, toID).Error; err != nil {
			return err
		}
		return tx.Exec(`
			UPDATE movements SET account_id = ?
			WHERE deleted_at IS NULL AND account_id = ?`, toID, fromID).Error
	})
}

type MovementQuery struct {
	UserID       uint64
	From         time.Time
	To           time.Time
	Currency     currency.Currency
	Type         *string
	AccountID    *uint64
	Category     *string
	Subcategory  *string
	Search       *string
	OnlyReserved bool
}

type CategorySum struct {
	Label string          `gorm:"column:label"`
	Total decimal.Decimal `gorm:"column:total"`
}

func (q MovementQuery) apply(db *gorm.DB) *gorm.DB {
	db = db.Where("movements.user_id = ? AND movements.currency = ?", q.UserID, q.Currency.String()).
		Where("movements.date >= ? AND movements.date <= ?",
			q.From.Format("2006-01-02"), q.To.Format("2006-01-02"))
	if q.Type != nil {
		db = db.Where("movements.type = ?", *q.Type)
	} else {
		db = db.Where("movements.type <> ?", string(constants.Transfer))
	}
	if q.AccountID != nil {
		db = db.Where("movements.account_id = ?", *q.AccountID)
	}
	if q.Category != nil {
		db = db.Where("s.category = ?", *q.Category)
	}
	if q.Subcategory != nil {
		db = db.Where("s.subcategory = ?", *q.Subcategory)
	}
	if q.Search != nil {
		t := "%" + *q.Search + "%"
		db = db.Where(
			"unaccent(lower(s.category)) LIKE unaccent(lower(?)) OR "+
				"unaccent(lower(s.subcategory)) LIKE unaccent(lower(?)) OR "+
				"unaccent(lower(coalesce(movements.description, ''))) LIKE unaccent(lower(?))",
			t, t, t)
	}
	switch {
	case q.OnlyReserved:
		db = db.Where("s.category IN ?", subcategory.ReservedCategories())
	case q.Type != nil && *q.Type == constants.Transfer:
		db = db.Where("s.category NOT IN ? OR (s.category = ? AND s.subcategory = ?)",
			subcategory.ReservedCategories(), subcategory.CategorySystem, subcategory.SubTransfer)
	default:
		db = db.Where("s.category NOT IN ?", subcategory.ReservedCategories())
	}
	return db
}

const (
	GroupByNone        = ""
	GroupByCategory    = "category"
	GroupBySubcategory = "subcategory"
	GroupByType        = "type"
	GroupByMonth       = "month"
	GroupByDay         = "day"
	GroupByAccount     = "account"
	GroupByDirection   = "direction"
)

func groupLabelExpr(groupBy string) string {
	switch groupBy {
	case GroupByCategory:
		return "s.category"
	case GroupBySubcategory:
		return "s.subcategory"
	case GroupByType:
		return "movements.type::text"
	case GroupByMonth:
		return "to_char(movements.date, 'YYYY-MM')"
	case GroupByDay:
		return "to_char(movements.date, 'YYYY-MM-DD')"
	case GroupByAccount:
		return "movements.account_id::text"
	case GroupByDirection:
		return "CASE WHEN movements.amount < 0 THEN 'out' ELSE 'in' END"
	default:
		return ""
	}
}

func (r *repository) SumForUser(q MovementQuery, groupBy string) ([]CategorySum, error) {
	var rows []CategorySum
	db := r.db.DB.Model(&Movement{}).
		Joins("JOIN subcategories s ON s.id = movements.subcategory_id")
	db = q.apply(db)

	label := groupLabelExpr(groupBy)
	if label == "" {
		db = db.Select("'' AS label, COALESCE(SUM(ABS(movements.amount)), 0) AS total")
	} else {
		db = db.Select(label + " AS label, COALESCE(SUM(ABS(movements.amount)), 0) AS total").
			Group(label).
			Order("total DESC")
	}
	err := db.Scan(&rows).Error
	return rows, err
}

func (r *repository) ListForUser(q MovementQuery, limit int) ([]Movement, error) {
	limit = clampListLimit(limit)
	var ms []Movement
	db := r.db.DB.Model(&Movement{}).
		Select("movements.*").
		Preload("Subcategory").
		Joins("JOIN subcategories s ON s.id = movements.subcategory_id")
	db = q.apply(db)
	err := db.Order("movements.date DESC, movements.id DESC").Limit(limit).Find(&ms).Error
	return ms, err
}

const (
	maxListLimit     = 50
	defaultListLimit = 20
)

func clampListLimit(limit int) int {
	if limit <= 0 || limit > maxListLimit {
		return defaultListLimit
	}
	return limit
}

func (r *repository) ListForAccount(accountID uint64, from, to time.Time, limit int) ([]Movement, error) {
	limit = clampListLimit(limit)
	var ms []Movement
	err := r.db.DB.Model(&Movement{}).
		Preload("Subcategory", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		Where("account_id = ?", accountID).
		Where("date >= ? AND date <= ?", from.Format("2006-01-02"), to.Format("2006-01-02")).
		Order("date DESC, id DESC").
		Limit(limit).
		Find(&ms).Error
	return ms, err
}

type DayCount struct {
	Date  time.Time `gorm:"column:date"`
	Count int       `gorm:"column:count"`
}

func (r *repository) CountByDayForUser(userID uint64, from, to time.Time) ([]DayCount, error) {
	var rows []DayCount
	err := r.db.DB.Model(&Movement{}).
		Select("movements.date AS date, COUNT(*) AS count").
		Where("movements.user_id = ?", userID).
		Where("movements.date >= ? AND movements.date <= ?", from.Format("2006-01-02"), to.Format("2006-01-02")).
		Group("movements.date").
		Scan(&rows).Error
	return rows, err
}

func (r *repository) TopExpenseForUser(q MovementQuery) (*Movement, error) {
	expense := string(Expense)
	q.Type = &expense
	var m Movement
	db := r.db.DB.Model(&Movement{}).
		Select("movements.*").
		Preload("Subcategory").
		Joins("JOIN subcategories s ON s.id = movements.subcategory_id")
	db = q.apply(db)
	err := db.Order("ABS(movements.amount) DESC, movements.id DESC").First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
