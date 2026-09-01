package orchestrator

import (
	"fmt"
	"strings"
)

func buildTaxonomyBlock(taxonomy []TaxonomyEntry) string {
	lines := make([]string, 0, len(taxonomy))
	for _, t := range taxonomy {
		line := fmt.Sprintf("%s | %s", t.Category, t.Subcategory)
		if t.Description != "" {
			line += " | " + t.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func buildAccountsBlock(accounts []AccountOption) string {
	lines := make([]string, 0, len(accounts))
	for _, a := range accounts {
		lines = append(lines, fmt.Sprintf("%d | %s (%s)", a.ID, a.Name, a.Currency))
	}
	return strings.Join(lines, "\n")
}
