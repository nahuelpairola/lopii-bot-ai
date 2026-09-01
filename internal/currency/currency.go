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

func (c Currency) Label() string {
	switch c {
	case ARS:
		return "pesos"
	case USD:
		return "dólares"
	default:
		return string(c)
	}
}

var SupportedCurrencies = []Currency{ARS, USD}
