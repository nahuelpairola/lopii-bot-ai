package subcategory

import "strings"

// ValidIcon is a minimal, deliberately imperfect check for
// stepNewCategoryIcon's free-text input: no existing Go stdlib
// emoji-range table, and no dependency already in go.mod gives one
// either — a real Unicode-emoji-property check needs a third-party
// package, not justified for a purely decorative field. This rejects
// the obvious bad cases (empty input, plain words, sentences) without
// pretending to be a perfect emoji validator.
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
