package nudges

import "lopiibot.com/internal/constants"

// Mensajes estáticos del dispatcher de tips, sin variables.
const (
	// Menú de preguntas (tip recurrente).
	msgMenuButton = "Preguntame"
	msgMenuHeader = "¿Qué querés saber?"
	msgMenuNoData = "Todavía no tengo suficiente cargado para sacar cuentas. Seguí anotando y en unos días te muestro."

	// msgQueryFailed vive en constants (QueryFailed) porque el borde
	// (controller/messaging) la manda con el mismo texto.
	msgQueryFailed = constants.QueryFailed
)
