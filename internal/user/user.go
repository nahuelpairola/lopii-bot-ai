package user

import (
	"time"

	"lopiibot.com/internal/database"
)

type User struct {
	ID        uint64    `gorm:"primaryKey"`
	Username  string    `gorm:"column:username"`
	IsAdmin   bool      `gorm:"column:is_admin"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// ChannelTelegram es el único lugar del modelo donde se nombra un canal.
// El core pasa este valor como identificador opaco: nadie ramifica sobre él.
const ChannelTelegram = "telegram"

// UserChannel es la dirección de un usuario en un canal. Una persona puede
// tener varias; el par (channel, channel_user_id) es único.
type UserChannel struct {
	ID            uint64    `gorm:"primaryKey"`
	UserID        uint64    `gorm:"column:user_id"`
	Channel       string    `gorm:"column:channel"`
	ChannelUserID string    `gorm:"column:channel_user_id"`
	CreatedAt     time.Time `gorm:"column:created_at"`
}

func (UserChannel) TableName() string { return "user_channels" }

type repository struct {
	conn *database.Connection
}

func NewRepository(conn *database.Connection) *repository {
	return &repository{conn: conn}
}

func (r *repository) FindByChannel(channel, channelUserID string) (*User, error) {
	var u User
	err := r.conn.DB.
		Joins("JOIN user_channels uc ON uc.user_id = users.id").
		Where("uc.channel = ? AND uc.channel_user_id = ?", channel, channelUserID).
		First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *repository) LinkChannel(userID uint64, channel, channelUserID string) error {
	return r.conn.DB.Create(&UserChannel{
		UserID:        userID,
		Channel:       channel,
		ChannelUserID: channelUserID,
	}).Error
}

func (r *repository) FindChannelID(userID uint64, channel string) (string, error) {
	var uc UserChannel
	err := r.conn.DB.
		Where("user_id = ? AND channel = ?", userID, channel).
		First(&uc).Error
	if err != nil {
		return "", err
	}
	return uc.ChannelUserID, nil
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
