package quote

import (
	"time"

	"gorm.io/gorm/clause"
	"lopiibot.com/internal/database"
)

// insertBatchSize acota el INSERT del sembrado: la carga inicial son ~15 años
// de historia por 7 casas, demasiadas filas para una sola sentencia.
const insertBatchSize = 1000

type repository struct{ conn *database.Connection }

func NewRepository(conn *database.Connection) *repository { return &repository{conn: conn} }

// LatestQuoteDate devuelve la fecha más nueva guardada, o nil si la tabla está
// vacía. Es lo único que el sweeper necesita para saber qué pedir.
func (r *repository) LatestQuoteDate() (*time.Time, error) {
	var dates []time.Time
	err := r.conn.DB.Model(&Quote{}).
		Order("date DESC").Limit(1).
		Pluck("date", &dates).Error
	if err != nil || len(dates) == 0 {
		return nil, err
	}
	return &dates[0], nil
}

// InsertQuotes inserta en lote, pisando lo que ya está. DoUpdates y no
// DoNothing a propósito: el valor del día en curso lo escribe dolarapi, que es
// provisorio, y el histórico de argentinadatos tiene que poder corregirlo
// cuando publica uno o dos días después. Con DoNothing ese valor provisorio
// quedaba fijo y la serie terminaba con dos fuentes mezcladas — un día con el
// valor de una y el siguiente con el de la otra, según si el bot estaba vivo a
// las 20:00. Reescribir el mismo valor es idempotente, así que el histórico
// pisándose a sí mismo no cuesta nada.
func (r *repository) InsertQuotes(qs []Quote) error {
	if len(qs) == 0 {
		return nil
	}
	return r.conn.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "date"}, {Name: "rate_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"bid", "ask"}),
	}).CreateInBatches(qs, insertBatchSize).Error
}

// InsertCPI inserta la lista entera cada vez. Son 53 KB y una sentencia por
// día: calcular qué meses faltan cuesta más que la query que lo calcula.
func (r *repository) InsertCPI(cs []CPI) error {
	if len(cs) == 0 {
		return nil
	}
	return r.conn.DB.Clauses(clause.OnConflict{DoNothing: true}).
		CreateInBatches(cs, insertBatchSize).Error
}
