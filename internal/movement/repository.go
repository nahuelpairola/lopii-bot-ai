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

// ErrNoMovementIDs: se pidió borrar sin decir qué. Es un bug del llamador, no
// un "no encontrado", y confundir los dos fue lo que dejó una corrección
// confirmada sin escribir el 2026-08-10.
var ErrNoMovementIDs = errors.New("movement: no ids to delete")

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

// CountForUser cuenta los movimientos no borrados del usuario. Barato, para
// condiciones de nudge (no mezcla monedas ni tipos — es un tally crudo).
func (r *repository) CountForUser(userID uint64) (int64, error) {
	var n int64
	err := r.db.DB.Model(&Movement{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}

// CountBySubcategory cuenta los movimientos VIVOS del usuario bajo una
// subcategoría (cualquier tipo, cualquier moneda, todo el tiempo). Cuenta solo
// vivos a propósito: es lo que el usuario ve en pantalla, y es el número que se
// le muestra antes de fusionar. MovementQuery no sirve acá — exige moneda y
// rango de fechas, y excluye transferencias por defecto.
func (r *repository) CountBySubcategory(userID uint64, subcategoryID uint64) (int64, error) {
	var n int64
	err := r.db.DB.Model(&Movement{}).
		Where("user_id = ? AND subcategory_id = ?", userID, subcategoryID).
		Count(&n).Error
	return n, err
}

// ReassignSubcategory mueve todos los movimientos de una subcategoría a otra.
// Es un UPDATE plano, sin la lógica de ReassignAccount: esa colapsa las
// transferencias entre las dos cuentas fusionadas, algo que no tiene sentido
// entre subcategorías.
//
// Unscoped a propósito: mueve también los movimientos soft-deleteados, para no
// dejar una fila borrada apuntando a una subcategoría que se está por borrar.
func (r *repository) ReassignSubcategory(userID uint64, fromID uint64, toID uint64) error {
	return r.db.DB.Unscoped().Model(&Movement{}).
		Where("user_id = ? AND subcategory_id = ?", userID, fromID).
		Update("subcategory_id", toID).Error
}

// TopDescriptionsBySubcategory devuelve las descripciones más frecuentes de
// una subcategoría, de la más usada a la menos. Alimenta el texto que se le
// manda al LLM para sugerir un destino de fusión: "Comida / Delivery — gastos
// en: PedidosYa, Rappi".
//
// Regresión aceptada del fold de merchant: los comercios se repetían
// ("Carrefour", "Carrefour") y las descripciones no ("18 mil pastas"), así que
// el top-5 es más ruidoso. Es una sugerencia, no un dato.
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

// SoftDeleteByIDs borra (soft-delete vía deleted_at) todas las filas
// listadas en un solo UPDATE.
//
// Borrar algo que YA estaba borrado NO es un error: el estado final es el que
// el usuario pidió. Antes se devolvía ErrMovementNotFound con 0 filas
// afectadas, y eso rompía dos caminos reales — un doble tap en el botón de
// confirmar, y el replay que hace pendingjob después de un 429. El 2026-08-10
// dos correcciones confirmadas murieron acá.
//
// Lo que sí es un error: que no exista NINGUNA de las filas pedidas
// (ErrMovementNotFound), o que no se pida ninguna (ErrNoMovementIDs).
func (r *repository) SoftDeleteByIDs(ids []uint) error {
	if len(ids) == 0 {
		return ErrNoMovementIDs
	}
	// Unscoped cuenta también las ya borradas: si la fila existe, el pedido
	// está satisfecho.
	var existing int64
	if err := r.db.DB.Unscoped().Model(&Movement{}).Where("id IN ?", ids).Count(&existing).Error; err != nil {
		return err
	}
	if existing == 0 {
		return ErrMovementNotFound
	}
	return r.db.DB.Where("id IN ?", ids).Delete(&Movement{}).Error
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

// MonthlyDelta is one signed month-over-month movement total for an
// account — the building block for a computed running balance. Delta is
// SIGNED (not ABS, unlike CategorySum) because a balance needs direction.
type MonthlyDelta struct {
	Month string          `gorm:"column:month"`
	Delta decimal.Decimal `gorm:"column:delta"`
}

// MonthlyDeltasForAccount returns every month with account activity,
// oldest first, with the signed sum of that month's movements. Callers
// cumsum this in Go over the FULL history (never a pre-filtered window) to
// avoid an opening-balance boundary bug, then slice the trailing N months
// for display.
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

// ReassignAccount mueve TODOS los movimientos vivos de la cuenta from a la
// cuenta to, en una sola transacción. NO es un UPDATE pelado: las
// transferencias internas from↔to (transacciones con una pata en cada una)
// se soft-deletean ENTERAS — netean 0 entre ambas cuentas, y re-apuntar la
// pata de from dejaría una "transferencia de to a to" que viola el
// invariante de 2 cuentas distintas (ver validateTransferGroups). El resto
// se re-apunta. Precondición (la valida el caller): misma moneda.
// ponytail: sin paginado — volúmenes de un usuario individual.
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

// MovementQuery is the shared filter for the read-only QUERY tools. One
// struct serves both SumForUser and ListForUser — identical filters, so a
// struct beats an 8-arg signature and keeps the two in sync. Type == nil
// means "exclude transfers" (the cash-flow default); a non-nil Type filters
// to exactly that type. Category/Subcategory/AccountID/Description are optional
// narrowing filters (Description is a substring ILIKE match — description is
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
	Description *string
	// OnlyReserved flips the reserved-category filter. The zero value — every
	// existing caller — EXCLUDES internal plumbing (opening balances, balance
	// adjustments, investment yield), because those are corrections to the
	// model, not money the user earned or spent: counting them as income or
	// expense double-counts a cause the app never recorded. Set it to true to
	// get ONLY those rows, which is how the balance-variation figure is built.
	//
	// This lives here, and not as a post-filter per view, because opt-in
	// filtering is what let `overview` and `summary` silently report balance
	// adjustments as real spending while `categories` and `evolution` hid them.
	OnlyReserved bool
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
	if q.Description != nil {
		db = db.Where("movements.description ILIKE ?", "%"+*q.Description+"%")
	}
	// Reserved categories are excluded by default and returned alone when
	// asked for — see MovementQuery.OnlyReserved. Both branches need the
	// subcategories join, which every apply() caller already does.
	if q.OnlyReserved {
		db = db.Where("s.category IN ?", subcategory.ReservedCategories())
	} else {
		db = db.Where("s.category NOT IN ?", subcategory.ReservedCategories())
	}
	return db
}

// GroupBy* are the recognized group_by keys for SumForUser. Exported so
// callers name the grouping instead of passing a magic string; groupLabelExpr
// is the single place that maps each key to its SQL expression.
const (
	GroupByNone        = ""
	GroupByCategory    = "category"
	GroupBySubcategory = "subcategory"
	GroupByType        = "type"
	GroupByMonth       = "month"
	GroupByDay         = "day"
	GroupByAccount     = "account"
)

// groupLabelExpr maps a group_by name to its SQL expression, or "" for none.
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

// maxListLimit / defaultListLimit acotan cuántas filas devuelve un listado. Un
// pedido fuera de rango cae al default en vez de errorear: estos listados
// alimentan vistas, y una vista sin filas es peor que una vista con 20.
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

// ListForAccount devuelve TODOS los movimientos de una cuenta en la ventana,
// más nuevo primero. A diferencia de ListForUser, NO pasa por
// MovementQuery.apply: la hoja de cuenta de la Mini App necesita justo lo que
// apply saca —transferencias y categorías reservadas— porque si no, la lista no
// cierra contra el saldo que esa vista muestra arriba. Meterle un tercer estado
// a los filtros de apply para servir a una vista arriesgaría a los otros cuatro
// consumidores; SumAmountForAccount y MonthlyDeltasForAccount ya lo esquivan por
// el mismo motivo.
//
// La ventana se bindea como STRING (ver el comentario de FindSimilarForUser):
// `date` es una columna DATE, y pasarle un time.Time hace que Postgres la
// castee con la timezone de la sesión y pierda las filas del día en curso.
func (r *repository) ListForAccount(accountID uint64, from, to time.Time, limit int) ([]Movement, error) {
	limit = clampListLimit(limit)
	var ms []Movement
	err := r.db.DB.Model(&Movement{}).
		// Unscoped a propósito, y sólo para MOSTRAR: una subcategoría borrada
		// sigue siendo el nombre correcto del movimiento que la usó, y un
		// Preload normal la devuelve nil (GORM respeta el soft delete), con lo
		// que una fila perfectamente identificable saldría como "Movimiento".
		// Esto NO contradice la regla de "sólo filas vivas": esa regla es para
		// ELEGIR una subcategoría —pickers, resolución de taxonomía—, donde
		// ofrecer una borrada sí es un bug.
		Preload("Subcategory", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		Where("account_id = ?", accountID).
		Where("date >= ? AND date <= ?", from.Format("2006-01-02"), to.Format("2006-01-02")).
		Order("date DESC, id DESC").
		Limit(limit).
		Find(&ms).Error
	return ms, err
}

// DayCount is one row of the per-day movement tally used by the weekly summary
// (activity metric). Count is all movements that day, any type/currency.
type DayCount struct {
	Date  time.Time `gorm:"column:date"`
	Count int       `gorm:"column:count"`
}

// CountByDayForUser tallies movements per calendar day in [from,to], across all
// currencies and types (activity signal, not money). deleted_at IS NULL is
// automatic (GORM soft delete).
// ponytail: counts rows; a grouped tx (transfer = 2 legs, batch = N rows) can
// inflate the tally. Fine for v1; dedupe by group if it matters.
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

// TopExpenseForUser returns the single largest expense (by absolute amount) for
// the query's window/currency, Subcategory preloaded (for category name).
// Returns (nil, nil) when there are no expenses. q.Type is forced to expense.
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
