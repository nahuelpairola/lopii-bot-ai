package subcategory

import (
	"slices"
	"strings"

	"lopiibot.com/internal/constants"
)

const CategorySystem = "Sistema"

const SubOpeningBalance = "Saldo inicial"

const SubTransfer = "Transferencia"

var reservedCategories = []string{CategorySystem, constants.PendingReview}

func ReservedCategories() []string {
	return slices.Clone(reservedCategories)
}

func IsReserved(name string) bool {
	name = strings.TrimSpace(name)
	for _, r := range reservedCategories {
		if strings.EqualFold(name, r) {
			return true
		}
	}
	return false
}
