package subcategory

import (
	"slices"
	"strings"
)

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

// SubTransfer es la subcategoría de Sistema donde viven las dos patas de cada
// transferencia entre cuentas propias. Exportada porque es la EXCEPCIÓN del
// filtro de reservadas en MovementQuery.apply: plata real del usuario, no
// plomería como SubOpeningBalance.
const SubTransfer = "Transferencia"

// reservedCategories are internal plumbing (opening balances, adjustment
// rows, the classification fallback) — never user-facing picker options,
// never proposable by the LLM.
var reservedCategories = []string{CategorySystem, "PENDING_REVIEW"}

// ReservedCategories returns the internal-plumbing category names, for the SQL
// filters that need them as a list rather than a per-row predicate (see
// movement.MovementQuery.apply). Returns a copy: the backing slice is
// process-wide, and a caller that appended to it would change what every other
// query treats as reserved.
func ReservedCategories() []string {
	return slices.Clone(reservedCategories)
}

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
