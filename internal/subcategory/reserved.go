package subcategory

import "strings"

// CategorySystem is the internal-plumbing category. Exported because the
// taxonomy lives here but `messaging` is what resolves rows under it (opening
// balances, balance adjustments, investment yield) — per the repo's
// no-duplicated-literal rule, the producing package owns the const.
//
// The name itself is seeded in migrations/; this is a reference to that row,
// not an invented name.
const CategorySystem = "Sistema"

// SubOpeningBalance is the "Sistema" subcategory every account's opening
// movement is filed under. Two call sites resolve it (account creation and the
// lazy first-account create inside movement_create), hence the const.
const SubOpeningBalance = "Saldo inicial"

// reservedCategories are internal plumbing (opening balances, adjustment
// rows, the classification fallback) — never user-facing picker options,
// never proposable by the LLM.
var reservedCategories = []string{CategorySystem, "PENDING_REVIEW"}

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
