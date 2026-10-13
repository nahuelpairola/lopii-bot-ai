package agent

import (
	"math"
	"testing"
)

func TestDescriptionCoverage(t *testing.T) {
	cases := []struct {
		name string
		desc string
		msg  string
		want float64
	}{
		{"three-letter description, whole word", "luz", "borrá la luz", 1},
		{"three-letter description, gym", "gym", "eliminá el gym", 1},
		{"all-short description from prod", "hbo max", "Elimina hbo max del 3 de octubre", 1},
		{"short word inside a longer description", "Gas departamento", "borrá el gas", 0.5},
		{"typo against the longer message word", "hbo max", "elimina hybo max", 0.75},
		{"half of the description named", "hbo max", "borrá lo de hbo", 0.5},
		{"three-letter typo against three-letter word does not count", "max", "borrá el de más", 0},
		{"three-letter token is a whole word, not a prefix", "gas", "el gasto de ayer", 0},
		{"three-letter token is not a substring of a longer word", "hbo max", "el máximo", 0},
		{"three-letter stopword overlap alone does not match", "cuota del gym", "borrá lo del super", 0},
		{"two-letter tokens never count", "de la", "borra el de la lista", 0},
		{"four-letter tokens keep matching as substrings", "panadería", "las panaderias de ayer", 1},
		{"typo in a long token", "panadería", "borrá la panadreia", 0.5},
		{"typo must keep the first letter", "nafta", "borrá la mafta", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := descriptionCoverage(tc.desc, tc.msg); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("descriptionCoverage(%q, %q) = %v, want %v", tc.desc, tc.msg, got, tc.want)
			}
		})
	}
}
