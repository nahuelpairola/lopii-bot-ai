package currency

import "lopiibot.com/internal/constants"

type Currency string

const (
	ARS Currency = constants.ARS
	USD Currency = constants.USD
)

func (c Currency) String() string {
	return string(c)
}

// Label es cómo se NOMBRA la moneda hablando, no su código ISO. "ARS" y "USD"
// son jerga contable: nadie dice "tengo 500 ARS".
//
// Vive acá por el mismo motivo que FormatMoney: nombrar una moneda es propio de
// la moneda, no del canal que la muestra. String() sigue siendo el código, y es
// el que va a la DB y a los `Value` de los botones — Label() es sólo para lo
// que lee una persona.
//
// En plural porque así entra en todas las frases donde aparece ("en pesos",
// "esos dólares", "movimientos en pesos").
func (c Currency) Label() string {
	switch c {
	case ARS:
		return "pesos"
	case USD:
		return "dólares"
	default:
		// Una moneda nueva sin traducir muestra su código: feo, pero legible.
		return string(c)
	}
}

var SupportedCurrencies = []Currency{ARS, USD}
