package subcategory

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"lopiibot.com/internal/database"
)

type Subcategory struct {
	ID          uint64    `gorm:"primaryKey"`
	UserID      *uint64   `gorm:"column:user_id"` // NULL para globales
	Category    string    `gorm:"column:category"`
	Subcategory string    `gorm:"column:subcategory"`
	Description string    `gorm:"column:description"`
	IsGlobal    bool      `gorm:"column:is_global"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

var ErrSubcategoryAlreadyExists = errors.New("a subcategory with that name already exists in this category")

type repository struct {
	conn *database.Connection
}

func NewRepository(conn *database.Connection) *repository {
	return &repository{conn: conn}
}

// FindAllForUser devuelve las propias del usuario + las globales del sistema.
func (r *repository) FindAllForUser(userID uint64) ([]Subcategory, error) {
	var subs []Subcategory
	err := r.conn.DB.
		Where("user_id = ? OR is_global = TRUE", userID).
		Find(&subs).Error
	return subs, err
}

// DistinctCategoriesForUser devuelve los nombres de categoría ya usados
// por el usuario (sin repetir), para ofrecerlos como opciones al crear
// una subcategoría nueva.
func (r *repository) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	var categories []string
	err := r.conn.DB.
		Model(&Subcategory{}).
		Where("user_id = ? OR is_global = TRUE", userID).
		Distinct("category").
		Order("category").
		Pluck("category", &categories).Error
	return categories, err
}

func (r *repository) Insert(s *Subcategory) error {
	err := r.conn.DB.Create(s).Error
	if isUniqueViolation(err) {
		return ErrSubcategoryAlreadyExists
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
