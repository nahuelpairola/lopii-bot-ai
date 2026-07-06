package subcategory

const (
	MsgInvalidCategoryName    = "Mandame un nombre válido para la categoría."
	MsgInvalidSubcategoryName = "Mandame un nombre válido para la subcategoría."
	MsgCreationError          = "No se pudo crear la subcategoría, probá de nuevo."

	BtnNewCategory = "➕ Otra categoría"
	BtnFinishSetup = "✅ Listo"

	MsgChooseCategoryIntro = "¿En qué categoría querés crear la subcategoría? " +
		"Elegí una existente o creá una nueva."

	MsgAskAddAnother = "Subcategoría creada ✅\n\n¿Querés agregar otra? Si ya terminaste, tocá \"Listo\"."
)

func MsgAskNewCategoryName() string {
	return "¿Cómo se llama la nueva categoría? (ej: Transporte, Ocio, Salud)"
}

func MsgAskSubcategoryName(category string) string {
	return "¿Cómo se llama la subcategoría dentro de \"" + category + "\"? (ej: Nafta, Cine, Farmacia)"
}

func MsgSubcategoryAlreadyExists(category, subcategory string) string {
	return "Ya existe \"" + subcategory + "\" en \"" + category + "\". Probá con otro nombre."
}
