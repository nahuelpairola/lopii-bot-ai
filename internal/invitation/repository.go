package invitation

import (
	"crypto/rand"
	"strings"
	"time"

	"lopiibot.com/internal/database"
)

const (
	expirationHours = 72
	codeCharset     = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // sin caracteres ambiguos (0,O,1,I)
	codeLength      = 6

	// listLimit topea la lista del admin. No hay paginación a propósito:
	// cincuenta invitaciones son años de un bot privado, y las que importan
	// mirar son siempre las últimas.
	listLimit = 50
)

type Invitation struct {
	ID        uint64     `gorm:"primaryKey"`
	Code      string     `gorm:"column:code;uniqueIndex"`
	CreatedBy uint64     `gorm:"column:created_by"`
	UsedBy    *uint64    `gorm:"column:used_by"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
}

func (Invitation) TableName() string {
	return "invitations"
}

type repository struct {
	conn *database.Connection
}

func NewRepository(conn *database.Connection) *repository {
	return &repository{conn: conn}
}

func (r *repository) FindByCode(code string) (*Invitation, error) {
	var inv Invitation
	if err := r.conn.DB.Where("code = ?", code).First(&inv).Error; err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *repository) Insert(inv *Invitation) error {
	return r.conn.DB.Create(inv).Error
}

func (r *repository) Create(createdBy uint64) (*Invitation, error) {
	code, err := generateCode()
	if err != nil {
		return nil, err
	}

	inv := &Invitation{
		Code:      code,
		CreatedBy: createdBy,
		ExpiresAt: time.Now().Add(expirationHours * time.Hour),
	}

	if err := r.Insert(inv); err != nil {
		return nil, err
	}
	return inv, nil
}

// List devuelve las invitaciones más recientes primero. Sin filtro por
// created_by: con un solo admin no acota nada, y con dos esconderían las del
// otro en la única vista que existe para verlas todas.
func (r *repository) List() ([]Invitation, error) {
	var invs []Invitation
	if err := r.conn.DB.Order("created_at desc").Limit(listLimit).Find(&invs).Error; err != nil {
		return nil, err
	}
	return invs, nil
}

func generateCode() (string, error) {
	b := make([]byte, codeLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, v := range b {
		sb.WriteByte(codeCharset[int(v)%len(codeCharset)])
	}
	return sb.String(), nil
}

func (r *repository) MarkAsUsed(id uint64, userID uint64) error {
	now := time.Now()
	return r.conn.DB.Model(&Invitation{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"used_by": userID,
			"used_at": now,
		}).Error
}
