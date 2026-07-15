package subcategory

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"lopiibot.com/internal/database"
)

type Subcategory struct {
	gorm.Model
	UserID      *uint64 `gorm:"column:user_id"` // NULL para globales
	Category    string  `gorm:"column:category"`
	Subcategory string  `gorm:"column:subcategory"`
	Description string  `gorm:"column:description"`
	IsGlobal    bool    `gorm:"column:is_global"`
	Icon        string  `gorm:"column:icon"`
}

var ErrSubcategoryAlreadyExists = errors.New("a subcategory with that name already exists in this category")
var ErrSubcategoryNotFound = errors.New("subcategory not found")

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
		Where("(user_id = ? OR is_global = TRUE) AND category NOT IN ?", userID, reservedCategories).
		Distinct("category").
		Order("category").
		Pluck("category", &categories).Error
	return categories, err
}

// FindByCategoryAndSubcategory busca una subcategoría específica (global
// o de usuario) por su par category+subcategory.
func (r *repository) FindByCategoryAndSubcategory(category, subcategory string) (*Subcategory, error) {
	var s Subcategory
	err := r.conn.DB.
		Where("category = ? AND subcategory = ?", category, subcategory).
		First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSubcategoryNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// FindAll devuelve absolutamente todas las subcategorías (globales y de
// usuario) — usada para poblar Cache una sola vez al arrancar el server,
// y de nuevo en cada Cache.Reload() tras un Insert (ver cache.go).
func (r *repository) FindAll() ([]Subcategory, error) {
	var subs []Subcategory
	err := r.conn.DB.Find(&subs).Error
	return subs, err
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
