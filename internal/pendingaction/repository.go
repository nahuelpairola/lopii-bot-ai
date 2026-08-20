package pendingaction

import (
	"errors"

	"gorm.io/gorm"
	"lopiibot.com/internal/database"
)

// ErrNoPendingAction es "no hay nada que drenar" — el caso normal, no un fallo.
var ErrNoPendingAction = errors.New("pendingaction: no pending action for user")

type repository struct {
	conn *database.Connection
}

func NewRepository(conn *database.Connection) *repository {
	return &repository{conn: conn}
}

func (r *repository) Insert(a *PendingAction) error {
	return r.conn.DB.Create(a).Error
}

// NextForUser devuelve la próxima acción a drenar: menor position, y a igual
// position la más vieja. El orden de dependencia lo fija quien parkea; el id
// sólo desempata.
func (r *repository) NextForUser(userID uint64) (*PendingAction, error) {
	var a PendingAction
	err := r.conn.DB.
		Where("user_id = ?", userID).
		Order("position, id").
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNoPendingAction
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Update reescribe la fila entera. Lo usa la re-búsqueda del picker, que pisa
// Payload y Questions y tiene que dejar intactos Position, Budget y TraceID —
// por eso es un Save de la fila y no un update de dos columnas.
func (r *repository) Update(a *PendingAction) error {
	return r.conn.DB.Save(a).Error
}

func (r *repository) Delete(id uint64) error {
	return r.conn.DB.Delete(&PendingAction{}, "id = ?", id).Error
}

func (r *repository) CountForUser(userID uint64) (int64, error) {
	var n int64
	err := r.conn.DB.Model(&PendingAction{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}
