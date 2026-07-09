package metric

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"lopiibot.com/internal/database"
)

// int64Array implements driver.Valuer so GORM binds it as one bigint[] arg
// instead of exploding it into an IN-list placeholder (its default behavior
// for any plain Go slice) — see Resolve.
type int64Array []int64

func (a int64Array) Value() (driver.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	strs := make([]string, len(a))
	for i, v := range a {
		strs[i] = fmt.Sprintf("%d", v)
	}
	return "{" + strings.Join(strs, ",") + "}", nil
}

// IntentEvent es una fila de intent_events: un mensaje clasificado por el
// router y su resultado end-to-end. CreatedAt lo autopopula GORM (campo
// llamado CreatedAt → autoCreateTime). ResolvedAt/WasCorrect son nullable.
type IntentEvent struct {
	ID                uint64     `gorm:"primaryKey"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UserID            uint64     `gorm:"column:user_id;not null"`
	RawMessage        string     `gorm:"column:raw_message;not null"`
	Intent            string     `gorm:"column:intent;not null"`
	NeedsConfirmation bool       `gorm:"column:needs_confirmation;not null"`
	Outcome           string     `gorm:"column:outcome;not null"`
	ResolvedAt        *time.Time `gorm:"column:resolved_at"`
	WasCorrect        *bool      `gorm:"column:was_correct"`
}

func (IntentEvent) TableName() string {
	return "intent_events"
}

type repository struct {
	db *database.Connection
}

func InitRepository(conn *database.Connection) *repository {
	return &repository{db: conn}
}

// Log inserta un evento nuevo. Los intents de movimiento entran con
// outcome="pending" y se resuelven después vía Resolve; el resto entra ya
// terminal.
//
// Antes de insertar, cierra como "abandoned" cualquier pending viejo del
// usuario: Log solo se llama en un mensaje libre sin flow activo (si hubiera
// flow, el engine lo maneja y no pasa por acá), así que un pending todavía
// abierto es huérfano — su flow murió sin llegar a un terminal. Sin esto,
// "último pending" (WIP=1) correlacionaría el próximo mensaje contra esa fila
// zombie en vez de la nueva.
// ponytail: dos statements, no una tx — una fila de métrica es fire-and-forget;
// una caída entre medio a lo sumo desetiqueta una fila de analítica, nunca
// datos del usuario.
func (r *repository) Log(userID uint64, rawMessage, intent string, needsConfirmation bool, outcome string) error {
	r.db.DB.Model(&IntentEvent{}).
		Where("user_id = ? AND outcome = ?", userID, "pending").
		Updates(map[string]interface{}{"outcome": "abandoned", "resolved_at": time.Now()})
	return r.db.DB.Create(&IntentEvent{
		UserID:            userID,
		RawMessage:        rawMessage,
		Intent:            intent,
		NeedsConfirmation: needsConfirmation,
		Outcome:           outcome,
	}).Error
}

// Resolve mueve el pending más reciente del usuario a un outcome terminal.
// "Más reciente" = mayor id (monotónico). Si no hay pending, es no-op sin
// error (0 filas afectadas). La invariante WIP=1 del engine garantiza que
// ese pending es la operación que está terminando ahora.
//
// ponytail: sin índice; agregar parcial (user_id, id) where outcome='pending'
// si la tabla crece.
func (r *repository) Resolve(userID uint64, outcome string, movementIDs []uint) error {
	now := time.Now()
	sub := r.db.DB.Model(&IntentEvent{}).
		Select("id").
		Where("user_id = ? AND outcome = ?", userID, "pending").
		Order("id DESC").
		Limit(1)
	updates := map[string]interface{}{"outcome": outcome, "resolved_at": now}
	if len(movementIDs) > 0 {
		// int64Array (driver.Valuer) so GORM binds one bigint[] arg instead of
		// exploding a plain slice into an IN-list. Omitted when empty so
		// cancelled/no-candidate rows keep movement_ids NULL.
		ids := make(int64Array, len(movementIDs))
		for i, id := range movementIDs {
			ids[i] = int64(id)
		}
		updates["movement_ids"] = ids
	}
	return r.db.DB.Model(&IntentEvent{}).
		Where("id = (?)", sub).
		Updates(updates).Error
}
