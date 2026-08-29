package templates

import (
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/currency"
)

var (
	thousandsFloor = decimal.NewFromInt(500)
	oneThousand    = decimal.NewFromInt(1_000)
)

func FormatMoney(d decimal.Decimal, cur currency.Currency) string {
	return currency.FormatMoney(d, cur)
}

func FormatCompact(d decimal.Decimal, cur currency.Currency) string {
	v := d.Abs()
	switch {
	case v.IsZero():
		return "·"
	case cur == currency.USD:
		return groupThousands(v.StringFixed(0))
	case v.LessThan(thousandsFloor):
		return "<1"
	default:
		return groupThousands(v.Div(oneThousand).StringFixed(0))
	}
}

func ScaleLabel(cur currency.Currency) string {
	if cur == currency.USD {
		return ""
	}
	return "en miles de $"
}

func groupThousands(s string) string { return currency.GroupThousands(s) }
