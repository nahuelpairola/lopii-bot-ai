package agent

// changeFieldOptions son los botones de la pregunta de qué cambiar. NO incluyen
// el monto a propósito: ese se escribe derecho y así el caso común —que es el
// monto— se resuelve en un paso. Los otros tres encadenan una segunda pregunta,
// porque tocar "la categoría" dice el campo pero no el valor.
//
// Sin emojis: el texto del botón se concatena al pedido, y un emoji ahí es
// ruido.
func changeFieldOptions() []string {
	return []string{labelChangeCategory, labelChangeDate, labelChangeAccount}
}

// Las etiquetas son consts porque se usan dos veces: para dibujar los botones y
// para traducir la respuesta de vuelta a un campo.
const (
	labelChangeCategory = "La categoría"
	labelChangeDate     = "La fecha"
	labelChangeAccount  = "La cuenta"
)

// changeFieldForLabel traduce el botón tocado al campo que nombra.
//
// Vive PEGADO a changeFieldOptions a propósito: si las dos listas divergen, el
// botón deja de construir la corrección, el pedido se cae al camino del modelo
// y nada lo avisa.
func changeFieldForLabel(label string) changeField {
	switch label {
	case labelChangeCategory:
		return fieldCategory
	case labelChangeDate:
		return fieldDate
	case labelChangeAccount:
		return fieldAccount
	default:
		return ""
	}
}
