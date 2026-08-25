package templates

import (
	"strconv"
	"strings"
	"time"
)

// RowFallbackTitle nombra una fila que no tiene ni descripción ni subcategoría.
// Sin esto la fila sale vacía y no le dice nada a nadie.
const RowFallbackTitle = "Movimiento"

// MovementRow es una fila de la lista de movimientos. Todo strings, como el
// resto de lo que reciben las vistas (ver AccountSnapshot): el handler formatea,
// el template sólo escribe.
type MovementRow struct {
	Icon  string
	Title string
	Date  string
	// Note es la aclaración al lado de la fecha. Casi siempre vacía; hoy sólo la
	// usa "Sin clasificar", para un movimiento que el LLM no supo encasillar.
	Note   string
	Amount string
}

// RowDate rinde la fecha de un movimiento como "12 ago".
//
// Toma año/mes/día DIRECTO, sin convertir de zona, y es deliberado: la fecha de
// un movimiento es una fecha CIVIL que se guarda como medianoche UTC, así que
// pasarla a ART la corre un día para atrás. Es el mismo motivo por el que existe
// civilDay en messaging/movement_label.go, visto en producción.
func RowDate(t time.Time) string {
	_, m, d := t.Date()
	return strconv.Itoa(d) + " " + monthShortEs[m-1]
}

// RowTitle es el nombre de la fila: la descripción, si no el nombre de la
// subcategoría, si no un genérico. Mismo orden de caída que candidateLabel en
// messaging — y los dos primeros pueden faltar: Description es *string, y
// Subcategory es un puntero que viene nil cuando la fila fue borrada.
func RowTitle(description *string, subcategory string) string {
	if description != nil {
		if s := strings.TrimSpace(*description); s != "" {
			return s
		}
	}
	if s := strings.TrimSpace(subcategory); s != "" {
		return s
	}
	return RowFallbackTitle
}

// MovementDayGroup es un día de la lista: el encabezado y las filas que caen
// abajo. La fecha sale a este nivel para que no se repita en las 50 filas.
type MovementDayGroup struct {
	Date string
	Rows []MovementRow
}

// GroupRowsByDay parte la lista en corridas CONSECUTIVAS de la misma fecha.
//
// Por corridas y no por un map, y no es un detalle: MovementRow.Date ya viene
// formateada por RowDate y NO lleva año, así que "12 ago" de 2026 y "12 ago"
// de 2025 son el mismo string — un map los metería en el mismo grupo, y el
// período "Año" llega a abarcar los dos. Recorrer la lista también conserva el
// orden que trae el repositorio, que es lo que hace que la hoja cierre contra
// el saldo leyéndola de arriba a abajo.
func GroupRowsByDay(rows []MovementRow) []MovementDayGroup {
	var groups []MovementDayGroup
	for _, row := range rows {
		if n := len(groups); n > 0 && groups[n-1].Date == row.Date {
			groups[n-1].Rows = append(groups[n-1].Rows, row)
			continue
		}
		groups = append(groups, MovementDayGroup{Date: row.Date, Rows: []MovementRow{row}})
	}
	return groups
}
