package messaging

import "lopiibot.com/internal/flow"

// parseUintSlice vive en flow (movement_text.go); el alias de acá lo usan los
// finishes de DELETE/UPDATE, que todavía viven en el borde.
func parseUintSlice(ids []string) ([]uint, error) {
	return flow.ParseUintSlice(ids)
}
