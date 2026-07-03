package movement

// IconForType devuelve el ícono de nivel-tipo heredado de v1: rojo para
// gasto, verde para ingreso, banco para cualquier otra cosa (transfer).
// No hay ícono por categoría acá — eso vive en subcategory.IconFor.
func IconForType(t movementType) string {
	switch t {
	case Expense:
		return "🔴"
	case Income:
		return "🟢"
	default:
		return "🏦"
	}
}

// TypeFromString maps a plain string (as returned by the orchestrator's
// MovementDraft.Type) to the package's movementType. Defaults to
// Expense for anything unrecognized — the taxonomy/prompt already
// constrains the LLM to "expense"/"income"/"transfer", so this is a
// safety net, not the primary validation.
func TypeFromString(s string) movementType {
	switch s {
	case string(Income):
		return Income
	case string(Transfer):
		return Transfer
	default:
		return Expense
	}
}
