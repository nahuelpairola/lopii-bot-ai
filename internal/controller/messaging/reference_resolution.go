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
	minMatchTokenLen      = 4
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
// candidate group: it matches when any description/merchant TOKEN of
// length >= 4 appears (case-insensitively) in the message, or the
// message literally contains one of the group's amounts. Token-level (not
// whole-phrase) so a verbose LLM description like "gasto en trabas"
// matches a message that shares only "trabas". The DB layer no longer
// pre-filters by similarity (see FindSimilarForUser / resolveCandidates),
// so this is the primary textual relevance check.
func matchesMessage(group transactionGroup, message string) bool {
	lower := strings.ToLower(message)
	for _, m := range group.Movements {
		if descOrMerchantTokenInMessage(m.Description, lower) {
			return true
		}
		if descOrMerchantTokenInMessage(m.Merchant, lower) {
			return true
		}
		if !m.Amount.IsZero() && strings.Contains(message, m.Amount.String()) {
			return true
		}
	}
	return false
}

// descOrMerchantTokenInMessage reports whether any whitespace-separated
// token of `field` with length >= minMatchTokenLen is a substring of the
// already-lowercased message.
func descOrMerchantTokenInMessage(field *string, lowerMessage string) bool {
	if field == nil || *field == "" {
		return false
	}
	for _, tok := range strings.Fields(strings.ToLower(*field)) {
		// ponytail: length>=4 skips es stopwords (de/en/el/con/por) without a
		// stopword list; standalone <=3-char descriptions like "pan"/"ypf"
		// won't match as tokens — revisit if that bites.
		if len([]rune(tok)) >= minMatchTokenLen {
			if strings.Contains(lowerMessage, tok) {
				return true
			}
		}
	}
	return false
}

// resolveCandidates finds the transaction group(s) a message could
// refer to — used for UPDATE/DELETE reference resolution and, with an
// empty dateFrom/dateTo, as CREATE's pre-insert duplicate check (see
// startMovementCreate). dateFrom/dateTo are whatever the orchestrator's
// resolve call extracted from the message — a single mentioned date
// sets only dateFrom; a range sets both. Either anchors the search
// window instead of the default 7-day cap.
func (c *controller) resolveCandidates(userID uint64, message, dateFrom, dateTo string) ([]transactionGroup, error) {
	since := time.Now().Add(-referenceSearchWindow)
	if dateFrom != "" {
		if anchor, err := time.Parse("2006-01-02", dateFrom); err == nil {
			since = anchor.Add(-dateAnchorMargin)
		}
	}

	var until *time.Time
	if dateTo != "" {
		if anchor, err := time.Parse("2006-01-02", dateTo); err == nil {
			u := anchor.Add(dateAnchorMargin)
			until = &u
		}
	}

	matches, err := c.movements.FindSimilarForUser(userID, message, since, until)
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
