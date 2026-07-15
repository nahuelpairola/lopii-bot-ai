package subcategory

import "strings"

// reservedCategories are internal plumbing (opening balances, adjustment
// rows, the classification fallback) — never user-facing picker options,
// never proposable by the LLM.
var reservedCategories = []string{"Sistema", "PENDING_REVIEW"}

// IsReserved reports whether name is an internal category, ignoring case
// and surrounding whitespace.
func IsReserved(name string) bool {
	name = strings.TrimSpace(name)
	for _, r := range reservedCategories {
		if strings.EqualFold(name, r) {
			return true
		}
	}
	return false
}
