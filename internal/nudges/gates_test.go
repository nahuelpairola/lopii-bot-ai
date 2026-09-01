package nudges

import (
	"testing"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/movement"
)

func daysAgo(n int, count int) movement.DayCount {
	return movement.DayCount{Date: agent.StartOfTodayArgentina().AddDate(0, 0, -n), Count: count}
}

func gateFor(t *testing.T, key string) func(Services, uint64, *nudgeStats) bool {
	t.Helper()
	for _, n := range nudges {
		if n.key == key {
			return n.when
		}
	}
	t.Fatalf("no existe el nudge %q", key)
	return nil
}

func TestNudgeStats_MovsSinceCountsMovementsNotDays(t *testing.T) {
	s := &nudgeStats{days: []movement.DayCount{
		daysAgo(0, 3),
		daysAgo(2, 2),
		daysAgo(9, 50),
	}}
	if got := s.movsSince(7); got != 5 {
		t.Errorf("movsSince(7) = %d, want 5", got)
	}
	if got := s.activeDaysSince(7); got != 2 {
		t.Errorf("activeDaysSince(7) = %d, want 2", got)
	}
}

func TestQueryTipGate_IgnoresOldMovements(t *testing.T) {
	svc := &testServices{counts: 4}

	stale := &nudgeStats{total: 4, days: []movement.DayCount{daysAgo(21, 4)}}
	if gateFor(t, nudgeQueryTip)(svc, 1, stale) {
		t.Error("query_tip no debería dispararse con movimientos de hace tres semanas")
	}

	fresh := &nudgeStats{total: 9, days: []movement.DayCount{daysAgo(1, 5), daysAgo(3, 4)}}
	if !gateFor(t, nudgeQueryTip)(svc, 1, fresh) {
		t.Error("query_tip debería dispararse con 9 movimientos en los últimos 7 días")
	}
}

func TestTopCategoryTipGate_NeedsDistinctCategories(t *testing.T) {
	cats := func(n int) *testServices {
		rows := make([]movement.CategorySum, n)
		for i := range rows {
			rows[i] = movement.CategorySum{Label: string(rune('A' + i))}
		}
		return &testServices{
			sumRows: func(q movement.MovementQuery, groupBy string) ([]movement.CategorySum, error) {
				return rows, nil
			},
		}
	}
	s := &nudgeStats{total: 20, days: []movement.DayCount{daysAgo(0, 10)}}

	if gateFor(t, nudgeTopCategoryTip)(cats(2), 1, s) {
		t.Error("top_category_tip no debería dispararse con 2 categorías")
	}
	if !gateFor(t, nudgeTopCategoryTip)(cats(3), 1, s) {
		t.Error("top_category_tip debería dispararse con 3 categorías")
	}
}

func TestActivityFloor_BlocksDormantUser(t *testing.T) {
	dormant := &nudgeStats{total: 40, days: []movement.DayCount{daysAgo(20, 40)}}
	if hasActivityFloor(dormant) {
		t.Error("un usuario sin cargas en 7 días no debería pasar el piso de actividad")
	}
}

func TestCompareTipGate_RespectsDayOfMonth(t *testing.T) {
	svc := &testServices{}
	s := &nudgeStats{
		total: 60,
		days: []movement.DayCount{
			daysAgo(0, 20),
			{Date: startOfMonth().AddDate(0, -1, 0), Count: 20},
			{Date: startOfMonth().AddDate(0, 0, -1), Count: 20},
		},
	}
	if s.movsInMonth(1) < compareTipMonthMovs {
		t.Fatalf("el fixture debería tener datos de sobra en el mes pasado, tiene %d", s.movsInMonth(1))
	}

	got := gateFor(t, nudgeCompareTip)(svc, 1, s)
	want := dayOfMonth() >= compareTipMinDay
	if got != want {
		t.Errorf("compare_tip = %v un día %d; con el umbral en %d debería ser %v",
			got, dayOfMonth(), compareTipMinDay, want)
	}
}
