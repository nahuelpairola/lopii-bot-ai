package flow

import (
	"strconv"
	"strings"
)

var accentFolder = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
)

func FoldAccents(s string) string { return accentFolder.Replace(s) }

const minMatchTokenLen = 4

func TokenCoverage(field, lowerHaystack string) float64 {
	total, hit := 0, 0
	for _, tok := range strings.Fields(FoldAccents(strings.ToLower(field))) {
		if len([]rune(tok)) < minMatchTokenLen {
			continue
		}
		total++
		if strings.Contains(lowerHaystack, tok) {
			hit++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

func TokenAppearsInString(field, lowerHaystack string) bool {
	return TokenCoverage(field, lowerHaystack) > 0
}

func GuessNamesOwnAccount(guess, description string) bool {
	if guess == "" {
		return false
	}
	return !TokenAppearsInString(guess, FoldAccents(strings.ToLower(description)))
}

func ParseUintSlice(ids []string) ([]uint, error) {
	out := make([]uint, 0, len(ids))
	for _, s := range ids {
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, uint(v))
	}
	return out, nil
}
