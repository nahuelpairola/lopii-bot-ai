package agent

import (
	"slices"
	"strconv"
	"strings"
	"unicode"

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

func transferAccountTrusted(id uint64, cur string, accounts []account.Account, msg string) bool {
	for _, a := range accounts {
		if uint64(a.ID) != id {
			continue
		}
		return a.Currency.String() == cur && (a.IsDefault || accountBackedByMessage(a, msg))
	}
	return false
}

func accountBackedByMessage(a account.Account, msg string) bool {
	haystack := foldAccents(strings.ToLower(msg))
	if haystack == "" {
		return false
	}
	if flow.TokenCoverage(a.Name, haystack) >= fullCoverage {
		return true
	}
	nameWords := strings.FieldsFunc(foldAccents(strings.ToLower(a.Name)), notWordRune)
	if len(nameWords) == 0 {
		return false
	}
	msgWords := make(map[string]bool)
	for _, w := range strings.FieldsFunc(haystack, notWordRune) {
		msgWords[w] = true
	}
	for _, w := range nameWords {
		if !msgWords[w] {
			return false
		}
	}
	return true
}

func defaultAccountID(accounts []account.Account, cur string) string {
	for _, a := range accounts {
		if a.IsDefault && a.Currency.String() == cur {
			return strconv.FormatUint(uint64(a.ID), 10)
		}
	}
	return ""
}

func soleAccountNamedInMessage(msg string, accounts []account.Account, cur string) uint64 {
	var found uint64
	for _, a := range accounts {
		if a.Currency.String() != cur || !accountBackedByMessage(a, msg) {
			continue
		}
		if found != 0 || nameExistsInOtherCurrency(a, accounts) {
			return 0
		}
		found = uint64(a.ID)
	}
	return found
}

func nameExistsInOtherCurrency(a account.Account, accounts []account.Account) bool {
	name := foldAccents(strings.ToLower(a.Name))
	for _, other := range accounts {
		if other.Currency != a.Currency && foldAccents(strings.ToLower(other.Name)) == name {
			return true
		}
	}
	return false
}

func groupCurrencyKey(group, cur string) string { return group + "|" + cur }

func legsPerGroupCurrency(drafts []orchestrator.MovementDraft) map[string]int {
	counts := make(map[string]int)
	for _, d := range drafts {
		if d.Type == constants.Transfer {
			counts[groupCurrencyKey(d.Group, d.Currency)]++
		}
	}
	return counts
}

func findAccount(accounts []account.Account, id uint64) (account.Account, bool) {
	for _, a := range accounts {
		if uint64(a.ID) == id {
			return a, true
		}
	}
	return account.Account{}, false
}

func sameInstitutionAsOtherLeg(rows []movement.MovementRow, i int, modelID uint64, defaulted map[int]uint64, accounts []account.Account) bool {
	mine, ok := findAccount(accounts, modelID)
	if !ok || mine.Currency.String() != rows[i].Currency {
		return false
	}
	for j, other := range rows {
		if _, pending := defaulted[j]; pending || j == i || other.Type != constants.Transfer || other.Group != rows[i].Group || other.AccountID == "" {
			continue
		}
		id, err := strconv.ParseUint(other.AccountID, 10, 64)
		if err != nil {
			continue
		}
		sibling, ok := findAccount(accounts, id)
		if ok && sibling.Currency != mine.Currency &&
			foldAccents(strings.ToLower(sibling.Name)) == foldAccents(strings.ToLower(mine.Name)) {
			return true
		}
	}
	return false
}

func defaultFitsTransferLeg(rows []movement.MovementRow, i int, defaulted map[int]uint64, accounts []account.Account) bool {
	def := defaultAccountID(accounts, rows[i].Currency)
	if def == "" {
		return false
	}
	for j, other := range rows {
		if j == i || other.Type != constants.Transfer || other.Group != rows[i].Group {
			continue
		}
		otherID := other.AccountID
		if _, pending := defaulted[j]; pending {
			otherID = defaultAccountID(accounts, other.Currency)
		}
		if otherID == def {
			return false
		}
	}
	return true
}

func notWordRune(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }

func buildCreateSeed(result orchestrator.CreateResult, taxonomy []orchestrator.TaxonomyEntry, accounts []account.Account, userText string) conversation.Data {
	rows := make([]movement.MovementRow, 0, len(result.Movements))
	var categoryGaps, accountGaps []string
	defaulted := make(map[int]uint64)
	legCounts := legsPerGroupCurrency(result.Movements)

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
		idx := strconv.Itoa(i)
		if draft.Category == constants.PendingReview || (len(known) > 0 && !known[draft.Category+"\x00"+draft.Subcategory]) {
			categoryGaps = append(categoryGaps, idx)
		}

		if draft.Type == constants.Transfer {
			named := uint64(0)
			if legCounts[groupCurrencyKey(draft.Group, draft.Currency)] == 1 {
				named = soleAccountNamedInMessage(userText, accounts, draft.Currency)
			}
			switch {
			case named != 0:
				row.AccountID = strconv.FormatUint(named, 10)
			case draft.AccountID == nil:
				if id := matchNamedAccount(draft.AccountNameGuess, accounts, draft.Currency); id != 0 {
					row.AccountID = strconv.FormatUint(id, 10)
				} else {
					accountGaps = append(accountGaps, idx)
				}
			case transferAccountTrusted(*draft.AccountID, draft.Currency, accounts, userText):
				row.AccountID = strconv.FormatUint(*draft.AccountID, 10)
			default:
				defaulted[i] = *draft.AccountID
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

	for i := range rows {
		if modelID, pending := defaulted[i]; pending && sameInstitutionAsOtherLeg(rows, i, modelID, defaulted, accounts) {
			rows[i].AccountID = strconv.FormatUint(modelID, 10)
			delete(defaulted, i)
		}
	}
	for i := range rows {
		if _, pending := defaulted[i]; pending && !defaultFitsTransferLeg(rows, i, defaulted, accounts) {
			accountGaps = append(accountGaps, strconv.Itoa(i))
		}
	}
	slices.SortFunc(accountGaps, func(a, b string) int {
		x, _ := strconv.Atoi(a)
		y, _ := strconv.Atoi(b)
		return x - y
	})

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
