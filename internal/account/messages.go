package account

import "lopiibot.com/internal/currency"

const (

	// El ejemplo usa coma decimal a propósito: es la convención argentina que
	// valida parseARAmount. "sin letras" es la parte que enseña por qué se
	// rechaza "30k". También cubre el caso de monto negativo (ver spec §3.2).
	MsgInvalidAmount = "Mandame solo el monto, sin letras (ej: 1500 o 1500,50)."
)

// Las dos nombran la moneda con currency.Label() —"pesos", "dólares"— y nunca
// con el código ISO: "ARS" es jerga contable y el usuario no la lee. El
// parámetro sigue llegando como código porque es lo que guarda el modelo.

func MsgAskInitialBalance(accountName, cur string) string {
	return "¿Cuánto tenés hoy en \"" + accountName + "\" (en " + currency.Currency(cur).Label() + ")? Tirame el número (ej: 50000)."
}

func MsgAccountAlreadyExists(name, cur string) string {
	return "Ya tenés una cuenta llamada \"" + name + "\" en " + currency.Currency(cur).Label() + ". Probá con otro nombre."
}
