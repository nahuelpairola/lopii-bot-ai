package flow

import (
	"math"
	"testing"
)

func TestTokenCoverage(t *testing.T) {
	cases := []struct {
		name       string
		field, msg string
		want       float64
	}{
		// El caso guía: la descripción coincide entera.
		{"descripción entera", "Débito tarjeta Mercado Pago",
			"del debito tarjeta mercado pago se me reintegraron $70.000", 1},
		// La misma frase genérica, pero sólo 2 de sus 3 tokens están.
		{"parcial", "Transferencia a Mercado Pago",
			"del debito tarjeta mercado pago se me reintegraron $70.000", 2.0 / 3.0},
		// Tokens de <4 runas no cuentan ni para el total ni para los aciertos.
		{"stopwords no cuentan", "de la", "borra el de la lista", 0},
		{"sin coincidencia", "compra en panaderia", "nafta en la estacion", 0},
		{"campo vacío", "", "cualquier cosa", 0},
	}
	for _, tc := range cases {
		if got := TokenCoverage(tc.field, tc.msg); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: TokenCoverage(%q, %q) = %v, want %v", tc.name, tc.field, tc.msg, got, tc.want)
		}
	}
}
