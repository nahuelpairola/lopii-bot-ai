package agent

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

func foldAccents(s string) string { return flow.FoldAccents(s) }

func StartOfTodayArgentina() time.Time {
	now := time.Now().In(constants.ArgentinaZone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, constants.ArgentinaZone)
}

const (
	dateAnchorMargin = 24 * time.Hour
	pickerMaxOptions = 5
	recencyLimit     = 60
	recencyWindow    = 90 * 24 * time.Hour
)

type transactionGroup struct {
	TransactionID string
	Movements     []movement.Movement
}

func groupByTransaction(ms []movement.Movement) []transactionGroup {
	order := make([]string, 0, len(ms))
	byKey := make(map[string][]movement.Movement)

	for _, m := range ms {
		key := fmt.Sprintf("single:%d", m.ID)
		if m.TransactionID != nil {
			key = m.TransactionID.String()
		}
		if _, seen := byKey[key]; !seen {
			order = append(order, key)
		}
		byKey[key] = append(byKey[key], m)
	}

	groups := make([]transactionGroup, 0, len(order))
	for _, key := range order {
		txID := key
		if strings.HasPrefix(key, "single:") {
			txID = ""
		}
		groups = append(groups, transactionGroup{TransactionID: txID, Movements: byKey[key]})
	}
	return groups
}

const dateTieBreak = 0.25

func scoreGroup(g transactionGroup, message string, anchor time.Time) float64 {
	lower := foldAccents(strings.ToLower(message))
	best := 0.0
	for _, m := range g.Movements {
		cover := 0.0
		if !m.Amount.IsZero() && strings.Contains(message, m.Amount.Abs().String()) {
			cover = 1
		}
		if m.Description != nil {
			if c := flow.TokenCoverage(*m.Description, lower); c > cover {
				cover = c
			}
		}
		if cover > best {
			best = cover
		}
	}
	if best == 0 {
		return 0
	}
	days := math.Abs(anchor.Sub(g.Movements[0].Date).Hours()) / 24
	return best + dateTieBreak/(1+days)
}

func matchesMessage(group transactionGroup, message string) bool {
	return scoreGroup(group, message, time.Now()) > 0
}

func dropReservedGroups(groups []transactionGroup) []transactionGroup {
	out := make([]transactionGroup, 0, len(groups))
	for _, g := range groups {
		if len(g.Movements) > 0 && g.Movements[0].Subcategory != nil &&
			subcategory.IsReserved(g.Movements[0].Subcategory.Category) {
			continue
		}
		out = append(out, g)
	}
	return out
}

func parseDateAnchor(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

func resolveCandidates(svc agentServices, userID uint64, message, dateFrom, dateTo string) ([]transactionGroup, error) {
	var matches []movement.Movement
	var err error
	if dateFrom == "" && dateTo == "" {
		matches, err = svc.FindRecentlyCreatedForUser(userID, time.Now().Add(-recencyWindow), recencyLimit)
	} else {
		from, to := parseDateAnchor(dateFrom), parseDateAnchor(dateTo)
		if from == nil {
			from = to
		}
		if to == nil {
			to = from
		}
		since := StartOfTodayArgentina()
		var until *time.Time
		if from != nil {
			since = from.Add(-dateAnchorMargin)
			u := to.Add(dateAnchorMargin)
			until = &u
		}
		matches, err = svc.MovementsFindSimilarForUser(userID, message, since, until)
	}
	if err != nil {
		return nil, err
	}

	groups := groupByTransaction(matches)

	anchor := StartOfTodayArgentina()
	if from := parseDateAnchor(dateFrom); from != nil {
		anchor = *from
	} else if to := parseDateAnchor(dateTo); to != nil {
		anchor = *to
	}

	type scored struct {
		group transactionGroup
		score float64
	}
	var candidates []scored
	for _, g := range groups {
		if s := scoreGroup(g, message, anchor); s > 0 {
			candidates = append(candidates, scored{group: g, score: s})
		}
	}
	if len(candidates) > 0 {
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].score > candidates[j].score
		})
		if len(candidates) > pickerMaxOptions {
			candidates = candidates[:pickerMaxOptions]
		}
		out := make([]transactionGroup, 0, len(candidates))
		for _, c := range candidates {
			out = append(out, c.group)
		}
		return out, nil
	}

	groups = dropReservedGroups(groups)

	if len(groups) > 0 && time.Since(groups[0].Movements[0].CreatedAt) <= flow.JustCreatedWindow {
		return groups[:1], nil
	}

	if len(groups) > pickerMaxOptions {
		groups = groups[:pickerMaxOptions]
	}
	return groups, nil
}
