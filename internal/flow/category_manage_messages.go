package flow

import "strings"

const (
	MsgCategoryManagePickSource    = "¿Cuál querés sacar?"
	MsgCategoryManageNoOwn         = "Solo puedo sacar las categorías que creaste vos — las que vienen de fábrica quedan siempre.\n\nTodavía no creaste ninguna. Si querés una nueva, pedímela: «creá una categoría para mascotas»."
	MsgCategoryManagePickTargetCat = "¿A qué categoría los mando?"
)

// CategoryLabel arma "Categoría › Subcategoría" con su ícono, la forma en que
// esta feature nombra una subcategoría en todos sus mensajes. Asume que el
// ícono ya viene resuelto (nunca ""): el único llamador de flujo 1 lo pasa vía
// SubcategoryIcon, que ya hizo el fallback.
func CategoryLabel(icon, category, subcategory string) string {
	return icon + " " + category + " › " + subcategory
}

func MsgCategoryManageSuggest(source, count, target string) string {
	return "«" + source + "» tiene " + count + " " + PluralMovimientos(count) + ".\n\n¿Los mando a «" + target + "»?"
}

func MsgCategoryManagePickTargetSub(category string) string {
	return "¿A qué subcategoría de " + category + "?"
}

func MsgCategoryManageConfirmDelete(source string) string {
	return "¿Borro «" + source + "»? No tenés movimientos ahí."
}

func MsgCategoryManageConfirmMerge(count, source, target string) string {
	return "Muevo " + count + " " + PluralMovimientos(count) + " de «" + source +
		"» a «" + target + "» y borro «" + source + "».\n\n¿Confirmo?"
}

func MsgCategoryManageDeleted(source string) string {
	return "Listo, saqué «" + source + "»."
}

func MsgCategoryManageMerged(count, source, target string) string {
	return "Listo: " + count + " " + PluralMovimientos(count) + " ahora están en «" +
		target + "», y «" + source + "» ya no está."
}

func PluralMovimientos(count string) string {
	if strings.TrimSpace(count) == "1" {
		return "movimiento"
	}
	return "movimientos"
}
