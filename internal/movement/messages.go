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
