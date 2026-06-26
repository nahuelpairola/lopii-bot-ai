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

var SupportedCurrencies = []Currency{ARS, USD}
