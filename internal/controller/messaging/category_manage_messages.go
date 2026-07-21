package messaging

import "strings"

const (
	msgCategoryManagePickSource    = "¿Cuál querés sacar?"
	msgCategoryManageNoOwn         = "Solo puedo sacar las categorías que creaste vos — las que vienen de fábrica quedan siempre.\n\nTodavía no creaste ninguna. Si querés una nueva, pedímela: «creá una categoría para mascotas»."
	msgCategoryManageCancelled     = "Listo, no toqué nada."
	msgCategoryManagePickTargetCat = "¿A qué categoría los mando?"
)

// categoryLabel arma "Categoría › Subcategoría" con su ícono, la forma en que
// esta feature nombra una subcategoría en todos sus mensajes. Asume que el
// ícono ya viene resuelto (nunca ""): el único llamador de flujo 1 lo pasa vía
// subcategoryIcon, que ya hizo el fallback.
func categoryLabel(icon, category, subcategory string) string {
	return icon + " " + category + " › " + subcategory
}

func msgCategoryManageSuggest(source, count, target string) string {
	return "«" + source + "» tiene " + count + " " + pluralMovimientos(count) + ".\n\n¿Los mando a «" + target + "»?"
}

func msgCategoryManagePickTargetSub(category string) string {
	return "¿A qué subcategoría de " + category + "?"
}

func msgCategoryManageConfirmDelete(source string) string {
	return "¿Borro «" + source + "»? No tenés movimientos ahí."
}

func msgCategoryManageConfirmMerge(count, source, target string) string {
	return "Muevo " + count + " " + pluralMovimientos(count) + " de «" + source +
		"» a «" + target + "» y borro «" + source + "».\n\n¿Confirmo?"
}

func msgCategoryManageDeleted(source string) string {
	return "Listo, saqué «" + source + "»."
}

func msgCategoryManageMerged(count, source, target string) string {
	return "Listo: " + count + " " + pluralMovimientos(count) + " ahora están en «" +
		target + "», y «" + source + "» ya no está."
}

func pluralMovimientos(count string) string {
	if strings.TrimSpace(count) == "1" {
		return "movimiento"
	}
	return "movimientos"
}
