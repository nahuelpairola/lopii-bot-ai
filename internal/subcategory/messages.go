package subcategory

const (
	msgInvalidCategoryName    = "Mandame un nombre válido para la categoría."
	msgInvalidSubcategoryName = "Mandame un nombre válido para la subcategoría."
	msgCreationError          = "No se pudo crear la subcategoría, probá de nuevo."

	btnNewCategory = "➕ Otra categoría"
	btnFinishSetup = "✅ Listo"

	msgChooseCategoryIntro = "¿En qué categoría querés crear la subcategoría? " +
		"Elegí una existente o creá una nueva."

	msgAskAddAnother = "Subcategoría creada ✅\n\n¿Querés agregar otra? Si ya terminaste, tocá \"Listo\"."
)

func msgAskNewCategoryName() string {
	return "¿Cómo se llama la nueva categoría? (ej: Transporte, Ocio, Salud)"
}

func msgAskSubcategoryName(category string) string {
	return "¿Cómo se llama la subcategoría dentro de \"" + category + "\"? (ej: Nafta, Cine, Farmacia)"
}

func msgSubcategoryAlreadyExists(category, subcategory string) string {
	return "Ya existe \"" + subcategory + "\" en \"" + category + "\". Probá con otro nombre."
}
