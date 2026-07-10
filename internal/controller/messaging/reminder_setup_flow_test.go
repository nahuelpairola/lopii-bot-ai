package messaging

import "testing"

func TestParseWindow(t *testing.T) {
	ok := []struct {
		in                 string
		wantStart, wantEnd int
	}{
		{"20 a 21", 1200, 1260},
		{"entre las 8 y las 10", 480, 600},
		{"de 9 a 13", 540, 780},
		{"20-22", 1200, 1320},
	}
	for _, c := range ok {
		s, e, err := parseWindow(c.in)
		if err != nil || s != c.wantStart || e != c.wantEnd {
			t.Errorf("parseWindow(%q) = (%d,%d,%v), want (%d,%d,nil)", c.in, s, e, err, c.wantStart, c.wantEnd)
		}
	}
	bad := []string{"", "21", "abc", "25 a 26", "21 a 20", "20 a 20", "-1 a 5"}
	for _, in := range bad {
		if _, _, err := parseWindow(in); err == nil {
			t.Errorf("parseWindow(%q) expected error, got nil", in)
		}
	}
}
