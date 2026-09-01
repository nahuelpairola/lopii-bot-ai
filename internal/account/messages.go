package account

import "lopiibot.com/internal/currency"

const (
	MsgInvalidAmount = "Mandame solo el monto, sin letras (ej: 1500 o 1500,50)."
)

func MsgAskInitialBalance(accountName, cur string) string {
	return "¿Cuánto tenés hoy en \"" + accountName + "\" (en " + currency.Currency(cur).Label() + ")? Tirame el número (ej: 50000)."
}

func MsgAccountAlreadyExists(name, cur string) string {
	return "Ya tenés una cuenta llamada \"" + name + "\" en " + currency.Currency(cur).Label() + ". Probá con otro nombre."
}
