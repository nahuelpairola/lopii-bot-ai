package flow

import (
	"strings"
	"time"

	"lopiibot.com/internal/movement"
)

const JustCreatedWindow = 10 * time.Minute

func FindNearDuplicate(inserted movement.Movement, sameTurnIDs []uint, priors []movement.Movement) *movement.Movement {
	excluded := make(map[uint]bool, len(sameTurnIDs))
	for _, id := range sameTurnIDs {
		excluded[id] = true
	}

	var byToken, byAmount *movement.Movement
	for i := range priors {
		p := &priors[i]
		if !nearDuplicateCandidate(inserted, *p, excluded) {
			continue
		}
		if sharesDescriptionToken(inserted, *p) {
			if byToken == nil || p.CreatedAt.After(byToken.CreatedAt) {
				byToken = p
			}
			continue
		}
		if byAmount == nil || p.CreatedAt.After(byAmount.CreatedAt) {
			byAmount = p
		}
	}
	if byToken != nil {
		return byToken
	}
	return byAmount
}

func nearDuplicateCandidate(m, p movement.Movement, excluded map[uint]bool) bool {
	if p.ID == m.ID || excluded[p.ID] {
		return false
	}
	if p.DeletedAt.Valid {
		return false
	}
	if p.UserID != m.UserID || p.Currency != m.Currency {
		return false
	}
	if p.Type != m.Type {
		return false
	}
	if !sameAccount(m.AccountID, p.AccountID) {
		return false
	}
	if m.TransactionID != nil || p.TransactionID != nil {
		return false
	}
	if m.CreatedAt.Sub(p.CreatedAt) > JustCreatedWindow || p.CreatedAt.After(m.CreatedAt) {
		return false
	}
	return sharesDescriptionToken(m, p) || sameAbsAmount(m, p)
}

func sameAccount(a, b *uint64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func sameAbsAmount(m, p movement.Movement) bool {
	return !m.Amount.IsZero() && m.Amount.Abs().Equal(p.Amount.Abs())
}

func sharesDescriptionToken(m, p movement.Movement) bool {
	if m.Description == nil || p.Description == nil {
		return false
	}
	lowered := FoldAccents(strings.ToLower(*p.Description))
	return TokenAppearsInString(*m.Description, lowered)
}

func NearDuplicateWindowStart(at time.Time) time.Time { return at.Add(-JustCreatedWindow) }
