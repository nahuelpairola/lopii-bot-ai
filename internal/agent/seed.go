package agent

import (
	"strconv"
	"strings"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

func guessNamesOwnAccount(guess, description string) bool {
	return flow.GuessNamesOwnAccount(guess, description)
}

const (
	modeCreate = flow.ModeCreate
	modeUpdate = flow.ModeUpdate
)

func matchNamedAccount(guess string, accounts []account.Account, cur string) uint64 {
	needle := foldAccents(strings.ToLower(strings.TrimSpace(guess)))
	if needle == "" {
		return 0
	}
	var found uint64
	for _, a := range accounts {
		if cur != "" && a.Currency.String() != cur {
			continue
		}
		if foldAccents(strings.ToLower(a.Name)) != needle {
			continue
		}
		if found != 0 {
			return 0
		}
		found = uint64(a.ID)
	}
	return found
}

func accountNamedInMessage(msg string, accounts []account.Account, cur string) (found uint64, ambiguous bool) {
	haystack := foldAccents(strings.ToLower(msg))
	if haystack == "" {
		return 0, false
	}
	for _, a := range accounts {
		if cur != "" && a.Currency.String() != cur {
			continue
		}
		if flow.TokenCoverage(a.Name, haystack) < fullCoverage {
			continue
		}
		if found != 0 {
			return 0, true
		}
		found = uint64(a.ID)
	}
	return found, false
}

func guessBackedByMessage(guess, msg string) bool {
	if guess == "" {
		return false
	}
	return flow.TokenCoverage(guess, foldAccents(strings.ToLower(msg))) >= fullCoverage
}

const fullCoverage = 1.0

func buildCreateSeed(result orchestrator.CreateResult, taxonomy []orchestrator.TaxonomyEntry, accounts []account.Account, userText string) conversation.Data {
	rows := make([]movement.MovementRow, 0, len(result.Movements))
	var categoryGaps, accountGaps []string

	known := make(map[string]bool, len(taxonomy))
	for _, t := range taxonomy {
		known[t.Category+"\x00"+t.Subcategory] = true
	}

	for i, draft := range result.Movements {
		row := movement.MovementRow{
			Type:             draft.Type,
			Amount:           draft.Amount,
			Currency:         draft.Currency,
			AccountNameGuess: draft.AccountNameGuess,
			Category:         draft.Category,
			Subcategory:      draft.Subcategory,
			Description:      draft.Description,
			Date:             draft.Date,
			Group:            draft.Group,
		}
		if draft.AccountID != nil && draft.Type == constants.Transfer {
			row.AccountID = strconv.FormatUint(*draft.AccountID, 10)
		}

		idx := strconv.Itoa(i)
		if draft.Category == constants.PendingReview || (len(known) > 0 && !known[draft.Category+"\x00"+draft.Subcategory]) {
			categoryGaps = append(categoryGaps, idx)
		}

		if draft.Type == constants.Transfer {
			if draft.AccountID == nil {
				if id := matchNamedAccount(draft.AccountNameGuess, accounts, draft.Currency); id != 0 {
					row.AccountID = strconv.FormatUint(id, 10)
				} else {
					accountGaps = append(accountGaps, idx)
				}
			}
		} else {
			named, ambiguous := accountNamedInMessage(userText, accounts, draft.Currency)
			switch {
			case ambiguous:
				accountGaps = append(accountGaps, idx)
			case named != 0:
				row.AccountID = strconv.FormatUint(named, 10)
			case guessBackedByMessage(draft.AccountNameGuess, userText) &&
				guessNamesOwnAccount(draft.AccountNameGuess, draft.Description):
				accountGaps = append(accountGaps, idx)
			}
		}

		rows = append(rows, row)
	}

	return conversation.Data{
		conversation.KeyMode:                modeCreate,
		conversation.KeyOldMovementIDs:      conversation.EncodeStringSlice(nil),
		conversation.KeyMovements:           movement.EncodeMovementRows(rows),
		conversation.KeyPendingCategoryGaps: conversation.EncodeStringSlice(categoryGaps),
		conversation.KeyPendingAccountGaps:  conversation.EncodeStringSlice(accountGaps),
	}
}

func categoryGapsFor(rows []movement.MovementRow, taxonomy []orchestrator.TaxonomyEntry) []string {
	if len(taxonomy) == 0 {
		return nil
	}
	known := make(map[string]bool, len(taxonomy))
	for _, t := range taxonomy {
		known[t.Category+"\x00"+t.Subcategory] = true
	}

	var gaps []string
	for i, r := range rows {
		if r.Category == constants.PendingReview || !known[r.Category+"\x00"+r.Subcategory] {
			gaps = append(gaps, strconv.Itoa(i))
		}
	}
	return gaps
}

func resolveTaxonomy(text string, taxonomy []orchestrator.TaxonomyEntry) (category, subcategory string, ok bool) {
	needle := normalizeForTaxonomy(text)
	if needle == "" {
		return "", "", false
	}

	var hits []orchestrator.TaxonomyEntry
	for _, t := range taxonomy {
		if normalizeForTaxonomy(t.Subcategory) == needle ||
			normalizeForTaxonomy(t.Category+" "+t.Subcategory) == needle {
			hits = append(hits, t)
		}
	}
	if len(hits) == 1 {
		return hits[0].Category, hits[0].Subcategory, true
	}
	if len(hits) == 0 {
		for _, t := range taxonomy {
			if normalizeForTaxonomy(t.Category) == needle {
				return t.Category, "", true
			}
		}
	}
	return "", "", false
}

func normalizeForTaxonomy(s string) string {
	s = foldAccents(strings.ToLower(s))
	for _, sep := range []string{"|", "/", ">", "-", ":"} {
		s = strings.ReplaceAll(s, sep, " ")
	}
	return strings.Join(strings.Fields(s), " ")
}

func accountGapsFor(rows []movement.MovementRow) []string {
	var gaps []string
	for i, r := range rows {
		if r.AccountID == "" && r.AccountNameGuess != "" {
			gaps = append(gaps, strconv.Itoa(i))
		}
	}
	return gaps
}
