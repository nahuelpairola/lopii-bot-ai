package user

import (
	"time"

	"lopiibot.com/internal/database"
)

type User struct {
	ID         uint64    `gorm:"primaryKey"`
	TelegramID string    `gorm:"column:telegram_id;uniqueIndex"`
	Username   string    `gorm:"column:username"`
	IsAdmin    bool      `gorm:"column:is_admin"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

type repository struct {
	conn *database.Connection
}

func NewRepository(conn *database.Connection) *repository {
	return &repository{conn: conn}
}

func (r *repository) FindByTelegramID(telegramID string) (*User, error) {
	var u User
	if err := r.conn.DB.Where("telegram_id = ?", telegramID).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *repository) FindByID(id uint64) (*User, error) {
	var u User
	if err := r.conn.DB.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *repository) Insert(u *User) error {
	return r.conn.DB.Create(u).Error
}
