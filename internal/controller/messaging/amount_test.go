package messaging

import (
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
)

func TestParseARAmount(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"45685,9", "45685.9", false},  // AR comma decimal — the incident value
		{"1.500,50", "1500.50", false}, // dots = thousands, comma = decimal
		{"123000", "123000", false},    // plain integer
		{"45685.9", "45685.9", false},  // already dot-normalized (LLM output)
		{"  2650 ", "2650", false},     // surrounding whitespace
		{"abc", "0", true},             // malformed
	}
	for _, c := range cases {
		got, err := parseARAmount(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseARAmount(%q): want error, got nil", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseARAmount(%q): unexpected error %v", c.in, err)
			continue
		}
		want := decimal.RequireFromString(c.want)
		if !got.Equal(want) {
			t.Errorf("parseARAmount(%q) = %s, want %s", c.in, got, want)
		}
	}
}

func TestParseARAmount_UserFormats(t *testing.T) {
	cases := []struct {
		in   string
		want string // decimal.String() esperado
		ok   bool
	}{
		{"2422,9", "2422.9", true},
		{"5694.08", "5694.08", true},
		{"1.000.000,50", "1000000.5", true},
		{"$5694.08", "5694.08", true},
		{"$ 2.422,90", "2422.9", true},
		{"5 694,08", "5694.08", true},
		{"1.000", "1000", true},
		{"1.000.000", "1000000", true},
		{"1.5", "1.5", true},
		{"pan", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, err := parseARAmount(c.in)
		if c.ok && (err != nil || got.String() != c.want) {
			t.Errorf("parseARAmount(%q) = %v, %v; want %s", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("parseARAmount(%q) expected error, got %v", c.in, got)
		}
	}
}

func TestParseARAmount_RejectsLetters(t *testing.T) {
	// Antes estas entradas se "limpiaban" en silencio ("100k" -> 100), lo que
	// producía un monto mal en un path de plata. Ahora tienen que fallar.
	for _, in := range []string{"30k", "100k", "10 mil", "1.5m", "100 xyz", "2 millones", "cien"} {
		if got, err := parseARAmount(in); err == nil {
			t.Errorf("parseARAmount(%q) = %s, want error", in, got)
		}
	}
}

func TestParseARAmount_AcceptsValidNumbers(t *testing.T) {
	cases := map[string]string{
		"300":          "300",
		"30000":        "30000",
		"435,68":       "435.68",
		"332.79":       "332.79",
		"$ 2.422,90":   "2422.9",
		"5 694,08":     "5694.08",
		"1.000.000,50": "1000000.5",
		"1.000":        "1000",
		"1.5":          "1.5",
	}
	for in, want := range cases {
		got, err := parseARAmount(in)
		if err != nil {
			t.Errorf("parseARAmount(%q): unexpected error %v", in, err)
			continue
		}
		if got.String() != want {
			t.Errorf("parseARAmount(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestValidateBalanceAmount_RejectsAbbreviation(t *testing.T) {
	// El hook que ven los flows: una abreviatura tiene que devolver el mensaje
	// de error, y un número limpio tiene que pasar.
	if msg := validateBalanceAmount("30k", nil); msg != account.MsgInvalidAmount {
		t.Errorf("validateBalanceAmount(%q) = %q, want %q", "30k", msg, account.MsgInvalidAmount)
	}
	if msg := validateBalanceAmount("30000", nil); msg != "" {
		t.Errorf("validateBalanceAmount(%q) = %q, want empty", "30000", msg)
	}
}
