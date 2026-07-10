package reminder

import "testing"

func TestMidpointMin(t *testing.T) {
	cases := []struct {
		start, end, want int
	}{
		{1200, 1320, 1260}, // 20:00–22:00 -> 21:00
		{1200, 1260, 1230}, // 20:00–21:00 -> 20:30 (sub-hour)
		{480, 600, 540},    // 08:00–10:00 -> 09:00
	}
	for _, c := range cases {
		r := Reminder{WindowStartMin: c.start, WindowEndMin: c.end}
		if got := r.MidpointMin(); got != c.want {
			t.Errorf("MidpointMin(%d,%d) = %d, want %d", c.start, c.end, got, c.want)
		}
	}
}
