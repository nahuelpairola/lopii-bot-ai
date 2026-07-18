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
