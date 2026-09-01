package nudges

import (
	"time"

	"lopiibot.com/internal/agent"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

const nudgeStatsWindow = 62

type nudgeStats struct {
	days  []movement.DayCount
	total int64
	sent  map[string]bool
}

func buildNudgeStats(s Services, userID uint64) *nudgeStats {
	st := &nudgeStats{}
	today := agent.StartOfTodayArgentina()
	st.days, _ = s.MovementsCountByDayForUser(userID, today.AddDate(0, 0, -nudgeStatsWindow), today)
	st.total, _ = s.MovementsCountForUser(userID)
	return st
}

func (s *nudgeStats) movsSince(n int) int {
	cutoff := agent.StartOfTodayArgentina().AddDate(0, 0, -n)
	total := 0
	for _, d := range s.days {
		if !d.Date.Before(cutoff) {
			total += d.Count
		}
	}
	return total
}

func (s *nudgeStats) activeDaysSince(n int) int {
	cutoff := agent.StartOfTodayArgentina().AddDate(0, 0, -n)
	days := 0
	for _, d := range s.days {
		if !d.Date.Before(cutoff) && d.Count > 0 {
			days++
		}
	}
	return days
}

func (s *nudgeStats) movsInMonth(monthsAgo int) int {
	start := startOfMonth().AddDate(0, -monthsAgo, 0)
	end := start.AddDate(0, 1, 0)
	total := 0
	for _, d := range s.days {
		if !d.Date.Before(start) && d.Date.Before(end) {
			total += d.Count
		}
	}
	return total
}

func startOfMonth() time.Time {
	today := agent.StartOfTodayArgentina()
	return time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
}

func dayOfMonth() int { return agent.StartOfTodayArgentina().Day() }

const (
	activityFloorMovs   = 3
	activityFloorDays   = 7
	recentTipMovs       = 5
	topCategoryTipMovs  = 8
	topCategoryTipCats  = 3
	balanceTipMovs      = 10
	balanceTipAccounts  = 2
	paceTipMovs         = 8
	paceTipActiveDays   = 4
	compareTipMonthMovs = 8

	compareTipMinDay = 10
	enoughTipMinDay  = 20
	paceTipMinDay    = 8
	paceTipMaxDay    = 25
)

func hasActivityFloor(s *nudgeStats) bool {
	return s.movsSince(activityFloorDays) >= activityFloorMovs
}

func distinctCategoriesThisMonth(s Services, userID uint64) int {
	rows, err := s.MovementsSumForUser(movement.MovementQuery{
		UserID:   userID,
		From:     startOfMonth(),
		To:       agent.StartOfTodayArgentina(),
		Currency: currency.ARS,
	}, "category")
	if err != nil {
		return 0
	}
	return len(rows)
}

func hasIncomeThisMonth(s Services, userID uint64) bool {
	income := string(movement.Income)
	rows, err := s.MovementsSumForUser(movement.MovementQuery{
		UserID:   userID,
		From:     startOfMonth(),
		To:       agent.StartOfTodayArgentina(),
		Currency: currency.ARS,
		Type:     &income,
	}, "none")
	return err == nil && len(rows) > 0 && !rows[0].Total.IsZero()
}

func hasEnoughDataForMonthVerdict(s Services, userID uint64, stats *nudgeStats) bool {
	return stats.movsInMonth(0) >= topCategoryTipMovs && hasIncomeThisMonth(s, userID)
}

func hasUsdHoldings(s Services, userID uint64) bool {
	accs, err := s.AccountsFindByUserID(userID)
	if err != nil {
		return false
	}
	for _, a := range accs {
		if a.Currency != currency.USD {
			continue
		}
		bal, err := s.MovementsSumAmountForAccount(uint64(a.ID))
		if err == nil && !bal.IsZero() {
			return true
		}
	}
	return false
}

func countAccounts(s Services, userID uint64) int {
	accs, err := s.AccountsFindByUserID(userID)
	if err != nil {
		return 0
	}
	return len(accs)
}
