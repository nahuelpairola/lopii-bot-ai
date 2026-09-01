package subcategory

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"lopiibot.com/internal/database"
)

type Subcategory struct {
	gorm.Model
	UserID      *uint64 `gorm:"column:user_id"`
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

func (r *repository) FindAllForUser(userID uint64) ([]Subcategory, error) {
	var subs []Subcategory
	err := r.conn.DB.
		Where("user_id = ? OR is_global = TRUE", userID).
		Find(&subs).Error
	return subs, err
}

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

func (r *repository) Delete(userID uint64, id uint64) error {
	result := r.conn.DB.
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&Subcategory{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSubcategoryNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
