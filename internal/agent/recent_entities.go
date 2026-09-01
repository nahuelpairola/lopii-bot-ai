package agent

import (
	"fmt"
	"strings"
	"time"

	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

const recentEntitiesCap = 8

func BuildRecentEntities(svc agentServices, userID uint64) string {
	movs, err := svc.FindRecentlyCreatedForUser(
		userID, StartOfTodayArgentina(), recentEntitiesCap)
	if err != nil {
		return ""
	}
	return renderRecentEntities(movs)
}

func renderRecentEntities(movs []movement.Movement) string {
	if len(movs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range movs {
		desc := ""
		if m.Description != nil {
			desc = strings.TrimSpace(*m.Description)
		}
		if desc == "" && m.Subcategory != nil {
			desc = m.Subcategory.Subcategory
		}
		account := ""
		if m.Account != nil {
			account = " · " + m.Account.Name
		}
		fmt.Fprintf(&b, "#%d  %s  %s%s  (%s)\n",
			m.ID, desc, currency.FormatMoney(m.Amount.Abs(), m.Currency), account, agoLabel(m.CreatedAt))
	}
	return strings.TrimRight(b.String(), "\n")
}

func agoLabel(at time.Time) string {
	d := time.Since(at)
	if d < time.Minute {
		return "recién"
	}
	return fmt.Sprintf("hace %d min", int(d.Minutes()))
}
