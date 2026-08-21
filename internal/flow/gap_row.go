package flow

import (
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

// ActiveGapRow es qué fila está contestando el gap de categoría ahora.
//
// La escribe stepResolveCategory al elegir, pero ese paso se saltea cuando la
// fila ya trae una categoría que existe: ahí el activo es el primer gap
// pendiente. Sin el fallback, saltearse la pregunta apuntaría a la fila 0 —
// otra fila, otro movimiento, la subcategoría puesta donde no va.
func ActiveGapRow(data conversation.Data) int {
	if raw := conversation.StringOrEmpty(data[conversation.KeyGapActiveRow]); raw != "" {
		idx, err := strconv.Atoi(raw)
		if err == nil {
			return idx
		}
	}
	gaps := conversation.DecodeStringSlice(data, conversation.KeyPendingCategoryGaps)
	if len(gaps) == 0 {
		return 0
	}
	idx, _ := strconv.Atoi(gaps[0])
	return idx
}

// rowCategoryExists dice si la fila del gap activo ya nombra una categoría que
// el usuario tiene. Sólo filas vivas: la taxonomía se resembró y los ids viejos
// siguen en la tabla, así que preguntarle al repo es lo único confiable.
func rowCategoryExists(subcategories subcategoryRepository, data conversation.Data) bool {
	rows := movement.DecodeMovementRows(data)
	idx := ActiveGapRow(data)
	if idx < 0 || idx >= len(rows) || rows[idx].Category == "" {
		return false
	}
	cats, err := subcategories.DistinctCategoriesForUser(data.UserID())
	if err != nil {
		return false
	}
	for _, c := range cats {
		if c == rows[idx].Category {
			return true
		}
	}
	return false
}
