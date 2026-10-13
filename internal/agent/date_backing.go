package agent

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"lopiibot.com/internal/constants"
)

const (
	maxDayDigits        = 2
	minMonthPrefixRunes = 3
)

var numericDayMonth = regexp.MustCompile(`(\d{1,2})\s*[/-]\s*(\d{1,2})`)

func dateBackedByMessage(message, date string) bool {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return false
	}
	day, month := d.Day(), int(d.Month())

	for _, m := range numericDayMonth.FindAllStringSubmatch(message, -1) {
		if atoi(m[1]) == day && atoi(m[2]) == month {
			return true
		}
	}

	monthName := constants.MonthLongEs[month-1]
	hasDay, hasMonth := false, false
	for _, tok := range matchTokens(message) {
		if len(tok) <= maxDayDigits && atoi(tok) == day {
			hasDay = true
		}
		if len([]rune(tok)) >= minMonthPrefixRunes && strings.HasPrefix(monthName, tok) {
			hasMonth = true
		}
	}
	return hasDay && hasMonth
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}

func messageBackedDate(message, date string) string {
	if dateBackedByMessage(message, date) {
		return date
	}
	return ""
}
