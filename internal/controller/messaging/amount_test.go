package messaging

import (
	"testing"

	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
)

func TestParseARAmount(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"45685,9", "45685.9", false},
		{"1.500,50", "1500.50", false},
		{"123000", "123000", false},
		{"45685.9", "45685.9", false},
		{"  2650 ", "2650", false},
		{"abc", "0", true},
	}
	for _, c := range cases {
		got, err := movement.ParseARAmount(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("movement.ParseARAmount(%q): want error, got nil", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("movement.ParseARAmount(%q): unexpected error %v", c.in, err)
			continue
		}
		want := decimal.RequireFromString(c.want)
		if !got.Equal(want) {
			t.Errorf("movement.ParseARAmount(%q) = %s, want %s", c.in, got, want)
		}
	}
}

func TestParseARAmount_UserFormats(t *testing.T) {
	cases := []struct {
		in   string
		want string
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
		got, err := movement.ParseARAmount(c.in)
		if c.ok && (err != nil || got.String() != c.want) {
			t.Errorf("movement.ParseARAmount(%q) = %v, %v; want %s", c.in, got, err, c.want)
		}
		if !c.ok && err == nil {
			t.Errorf("movement.ParseARAmount(%q) expected error, got %v", c.in, got)
		}
	}
}

func TestParseARAmount_RejectsLetters(t *testing.T) {
	for _, in := range []string{"30k", "100k", "10 mil", "1.5m", "100 xyz", "2 millones", "cien"} {
		if got, err := movement.ParseARAmount(in); err == nil {
			t.Errorf("movement.ParseARAmount(%q) = %s, want error", in, got)
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
		got, err := movement.ParseARAmount(in)
		if err != nil {
			t.Errorf("movement.ParseARAmount(%q): unexpected error %v", in, err)
			continue
		}
		if got.String() != want {
			t.Errorf("movement.ParseARAmount(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestValidateBalanceAmount_RejectsAbbreviation(t *testing.T) {
	if msg := flow.ValidateBalanceAmount("30k", nil); msg != account.MsgInvalidAmount {
		t.Errorf("flow.ValidateBalanceAmount(%q) = %q, want %q", "30k", msg, account.MsgInvalidAmount)
	}
	if msg := flow.ValidateBalanceAmount("30000", nil); msg != "" {
		t.Errorf("flow.ValidateBalanceAmount(%q) = %q, want empty", "30000", msg)
	}
}
