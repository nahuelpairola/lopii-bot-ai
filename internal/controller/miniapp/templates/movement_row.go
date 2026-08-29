package templates

import (
	"strconv"
	"strings"
	"time"
)

const RowFallbackTitle = "Movimiento"

type MovementRow struct {
	Icon  string
	Title string
	Date  string

	Note   string
	Amount string
}

func RowDate(t time.Time) string {
	_, m, d := t.Date()
	return strconv.Itoa(d) + " " + monthShortEs[m-1]
}

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

type MovementDayGroup struct {
	Date string
	Rows []MovementRow
}

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
