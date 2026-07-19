package messaging

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestParseARAmount(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"45685,9", "45685.9", false},   // AR comma decimal — the incident value
		{"1.500,50", "1500.50", false},  // dots = thousands, comma = decimal
		{"123000", "123000", false},     // plain integer
		{"45685.9", "45685.9", false},   // already dot-normalized (LLM output)
		{"  2650 ", "2650", false},      // surrounding whitespace
		{"abc", "0", true},              // malformed
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
