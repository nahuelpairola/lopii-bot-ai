package templates

import (
	"strings"

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

func TrendChartAlt(subject string, c TrendChartData, cur currency.Currency) string {
	if len(c.Labels) == 0 || len(c.Datasets) == 0 {
		return "Gráfico de " + subject + ", sin datos."
	}

	money := func(v float64) string {
		return FormatMoney(decimal.NewFromFloat(v), cur)
	}

	var b strings.Builder
	b.WriteString("Gráfico de " + subject + ", ")
	if len(c.Labels) == 1 {
		b.WriteString("en " + c.Labels[0] + ".")
	} else {
		b.WriteString("de " + c.Labels[0] + " a " + c.Labels[len(c.Labels)-1] + ".")
	}

	for _, d := range c.Datasets {
		switch {
		case len(d.Data) == 0:
			continue
		case len(d.Data) == 1:
			b.WriteString(" " + d.Label + ": " + money(d.Data[0]) + ".")
		default:
			peak := 0
			for i, v := range d.Data {
				if v > d.Data[peak] {
					peak = i
				}
			}
			b.WriteString(" " + d.Label + ": de " + money(d.Data[0]) +
				" a " + money(d.Data[len(d.Data)-1]) +
				", máximo " + money(d.Data[peak]))
			if peak < len(c.Labels) {
				b.WriteString(" en " + c.Labels[peak])
			}
			b.WriteString(".")
		}
	}

	return b.String()
}

func TableLabel(header string) string {
	return "Gastos por " + strings.ToLower(header)
}
