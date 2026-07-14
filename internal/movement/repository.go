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
	Merchant      *string                  `gorm:"column:merchant"`
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

// AccountOpening pairs an account to create with its opening movement. The
// movement's AccountID is filled in by InsertAccountsWithOpenings after the
// account is created (its ID isn't known until then).
type AccountOpening struct {
	Account  *account.Account
	Movement Movement
}

// InsertAccountsWithOpenings creates every account and its opening movement
// in one transaction: a mid-insert failure rolls back all of them, so
// onboarding never leaves a user with half their accounts. Cross-repo on
// purpose — both accounts and movements wrap the same *database.Connection,
// so one db.Transaction covers both.
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

// FindSimilarForUser returns the user's non-deleted movements in the
// [since, until] date window (until nil = no upper bound), ordered most
// recent first, with the subcategory preloaded. Despite the name it no
// longer does a pg_trgm similarity filter: that DB-side filter dropped
// short-description-in-long-message matches (see the humane-reference-
// resolution spec). Textual relevance is now decided in-process by
// matchesMessage (see reference_resolution.go), so `query` is unused here.
// The window is already narrow (one day by default, per user), so a plain
// scan is cheap.
// ponytail: `query` param + the name are legacy; a rename is a safe
// follow-up but would ripple through the interface and three test mocks.
//
// since/until are formatted to "YYYY-MM-DD" before binding: movements.date
// is a plain DATE column (no timezone), while since/until are ART-offset
// timestamps. Comparing a DATE column directly against a timestamptz
// parameter makes Postgres cast the column to timestamptz using the
// session's own timezone (UTC on this server), not the ART offset carried
// by the parameter — every "today" row's midnight-UTC cast then falls
// before an ART-anchored `since`, silently excluding it. Binding a date
// string instead is a plain DATE-to-DATE comparison with no cast involved.
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

// FindRecentlyCreatedForUser returns the user's non-deleted movements
// RECORDED (created_at) at or after `since`, newest-recorded first,
// subcategory preloaded. This is the "what did I just do" window for a
// correction/deletion that names no date: recency of ENTRY, not of the
// movement's business date — a movement entered today but dated in the
// past ("le pagué el asado de ayer") must still be a candidate.
// created_at is timestamptz, so a time.Time binds directly (no DATE-cast
// trap; see FindSimilarForUser's note on why `date` needs a string bind).
func (r *repository) FindRecentlyCreatedForUser(userID uint64, since time.Time) ([]Movement, error) {
	var ms []Movement
	err := r.db.DB.Preload("Subcategory").Preload("Account").
		Where("user_id = ? AND created_at >= ?", userID, since).
		Order("created_at DESC, id DESC").
		Find(&ms).Error
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

// SoftDeleteByUserID soft-deletes every movement of the user (reset). Unlike
// SoftDeleteByIDs it does not error on zero rows — a user with no movements is
// a valid reset target.
func (r *repository) SoftDeleteByUserID(userID uint64) error {
	return r.db.DB.Where("user_id = ?", userID).Delete(&Movement{}).Error
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

// MovementQuery is the shared filter for the read-only QUERY tools. One
// struct serves both SumForUser and ListForUser — identical filters, so a
// struct beats an 8-arg signature and keeps the two in sync. Type == nil
// means "exclude transfers" (the cash-flow default); a non-nil Type filters
// to exactly that type. Category/Subcategory/AccountID/Merchant are optional
// narrowing filters (Merchant is a substring ILIKE match — merchant is
// pg_trgm-indexed). Currency is always required — ARS and USD are never mixed.
type MovementQuery struct {
	UserID      uint64
	From        time.Time
	To          time.Time
	Currency    currency.Currency
	Type        *string
	Category    *string
	Subcategory *string
	AccountID   *uint64
	Merchant    *string
}

// CategorySum is one grouped aggregate row. Label is the group key (category
// name, subcategory, type, account_id as text, "YYYY-MM", "YYYY-MM-DD", or
// "" when group_by is none). Total is SUM(ABS(amount)) — the sign is a
// storage detail and never surfaces.
type CategorySum struct {
	Label string          `gorm:"column:label"`
	Total decimal.Decimal `gorm:"column:total"`
}

// apply adds the shared WHERE clauses to a query already joined to
// subcategories (alias s). deleted_at IS NULL is automatic (GORM soft delete).
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
	if q.Merchant != nil {
		db = db.Where("movements.merchant ILIKE ?", "%"+*q.Merchant+"%")
	}
	return db
}

// groupLabelExpr maps a group_by name to its SQL expression, or "" for none.
func groupLabelExpr(groupBy string) string {
	switch groupBy {
	case "category":
		return "s.category"
	case "subcategory":
		return "s.subcategory"
	case "type":
		return "movements.type::text"
	case "month":
		return "to_char(movements.date, 'YYYY-MM')"
	case "day":
		return "to_char(movements.date, 'YYYY-MM-DD')"
	case "account":
		return "movements.account_id::text"
	default:
		return ""
	}
}

// SumForUser returns SUM(ABS(amount)) over the filtered movements, optionally
// grouped. group_by "" (or unknown) yields a single total. Invariants baked
// in: user-scoped, single currency, abs amounts, transfer excluded by default.
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

// ListForUser returns the filtered movements newest-first, capped. Subcategory
// is preloaded so callers can render category/subcategory names. Amounts are
// stored signed; callers must render Amount.Abs().
func (r *repository) ListForUser(q MovementQuery, limit int) ([]Movement, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var ms []Movement
	db := r.db.DB.Model(&Movement{}).
		Select("movements.*").
		Preload("Subcategory").
		Joins("JOIN subcategories s ON s.id = movements.subcategory_id")
	db = q.apply(db)
	err := db.Order("movements.date DESC, movements.id DESC").Limit(limit).Find(&ms).Error
	return ms, err
}
