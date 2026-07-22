package messaging

import (
	"fmt"
	"strings"
	"time"

	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/movement"
)

func startOfTodayArgentina() time.Time {
	now := time.Now().In(constants.ArgentinaZone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, constants.ArgentinaZone)
}

const (
	dateAnchorMargin  = 24 * time.Hour
	minMatchTokenLen  = 4
	fallbackRecentCap = 5
	// ponytail: 48h covers same-session + "recorded last night, fixing this
	// morning" without naming a date; widen if corrections routinely lag longer.
	recencyWindow = 48 * time.Hour
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

// resolveCandidates finds the transaction group(s) a message could refer
// to for UPDATE/DELETE. Default window is today (America/Argentina/
// Buenos_Aires); a mentioned dateFrom/dateTo anchors it instead. It fetches
// the whole window (see FindSimilarForUser) and matches in-process via
// matchesMessage. When nothing matches textually it does NOT dead-end —
// it returns the window's most-recent groups (capped) so the caller can
// ask "¿cuál?". It never auto-picks: the caller still confirms (1) or
// shows a picker (2+).
func (c *controller) resolveCandidates(userID uint64, message, dateFrom, dateTo string) ([]transactionGroup, error) {
	var matches []movement.Movement
	var err error
	if dateFrom == "" && dateTo == "" {
		// No date named → "what did I just do": recency of ENTRY (created_at),
		// not business date. A movement entered now but dated in the past
		// ("le pagué el asado de ayer") must still be a candidate.
		matches, err = c.movements.FindRecentlyCreatedForUser(userID, time.Now().Add(-recencyWindow))
	} else {
		since := startOfTodayArgentina()
		if dateFrom != "" {
			if anchor, perr := time.Parse("2006-01-02", dateFrom); perr == nil {
				since = anchor.Add(-dateAnchorMargin)
			}
		}
		var until *time.Time
		if dateTo != "" {
			if anchor, perr := time.Parse("2006-01-02", dateTo); perr == nil {
				u := anchor.Add(dateAnchorMargin)
				until = &u
			}
		}
		matches, err = c.movements.FindSimilarForUser(userID, message, since, until)
	}
	if err != nil {
		return nil, err
	}

	groups := groupByTransaction(matches)

	var candidates []transactionGroup
	for _, g := range groups {
		if matchesMessage(g, message) {
			candidates = append(candidates, g)
		}
	}
	if len(candidates) > 0 {
		return candidates, nil
	}

	// Nothing matched textually. Rather than dead-end, offer the most
	// recent movements in the window as a picker. groups is already ordered
	// newest-first by the window query (created_at DESC in the default
	// no-date path, date DESC when a date was mentioned).
	if len(groups) > fallbackRecentCap {
		groups = groups[:fallbackRecentCap]
	}
	return groups, nil
}
