package messaging

import (
	"fmt"
	"strings"
	"time"

	"lopiibot.com/internal/movement"
)

const (
	referenceSearchWindow = 7 * 24 * time.Hour
	dateAnchorMargin      = 24 * time.Hour
)

// transactionGroup is a set of movements sharing one transaction_id (or
// a single standalone movement without one) — the unit every UPDATE/
// DELETE reference-resolution step operates on, never a lone row.
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

// matchesMessage is a cheap, dependency-free relevance filter over one
// candidate group: case-insensitive substring match against
// description/merchant, or the message literally containing one of the
// group's amounts. It's deliberately loose — pg_trgm has already
// narrowed the DB-side search (see resolveCandidates); this is a final
// in-process pass, not the primary filter.
func matchesMessage(group transactionGroup, message string) bool {
	lower := strings.ToLower(message)
	for _, m := range group.Movements {
		if m.Description != nil && *m.Description != "" && strings.Contains(lower, strings.ToLower(*m.Description)) {
			return true
		}
		if m.Merchant != nil && *m.Merchant != "" && strings.Contains(lower, strings.ToLower(*m.Merchant)) {
			return true
		}
		if !m.Amount.IsZero() && strings.Contains(message, m.Amount.String()) {
			return true
		}
	}
	return false
}

// resolveCandidates finds the transaction group(s) an UPDATE/DELETE
// message could refer to, once the lastTransaction check has already
// come back unresolved (or there was no lastTransaction to check).
// mentionedDate is whatever the orchestrator's resolve call extracted
// from the message, if any — it anchors the search window instead of
// the default 7-day cap, per the spec's dynamic-search rule.
func (c *controller) resolveCandidates(userID uint64, message, mentionedDate string) ([]transactionGroup, error) {
	since := time.Now().Add(-referenceSearchWindow)
	if mentionedDate != "" {
		if anchor, err := time.Parse("2006-01-02", mentionedDate); err == nil {
			since = anchor.Add(-dateAnchorMargin)
		}
	}

	matches, err := c.movements.FindSimilarForUser(userID, message, since)
	if err != nil {
		return nil, err
	}

	var candidates []transactionGroup
	for _, g := range groupByTransaction(matches) {
		if matchesMessage(g, message) {
			candidates = append(candidates, g)
		}
	}
	return candidates, nil
}
