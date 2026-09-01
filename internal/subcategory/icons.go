package subcategory

import "strings"

func ValidIcon(s string) bool {
	if s == "" || strings.Contains(s, " ") {
		return false
	}
	if len([]rune(s)) > 4 {
		return false
	}
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}
