package flow

import (
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
)

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
