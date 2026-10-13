package agent

import (
	"strings"
	"unicode"
)

const (
	minCountedTokenRunes   = 3
	minSubstringTokenRunes = 4
	minTypoLongerRunes     = 4
	maxTypoDistance        = 1
	fuzzyHitWeight         = 0.5
)

var shortStopwords = map[string]bool{
	"del": true, "los": true, "las": true, "con": true, "por": true, "una": true, "uno": true,
	"que": true, "mis": true, "sus": true, "sin": true, "mes": true, "dia": true, "hoy": true,
}

func matchTokens(s string) []string {
	return strings.FieldsFunc(foldAccents(strings.ToLower(s)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func descriptionCoverage(description, message string) float64 {
	lowerMsg := foldAccents(strings.ToLower(message))
	words := matchTokens(message)

	total, hits := 0, 0.0
	for _, tok := range matchTokens(description) {
		runes := len([]rune(tok))
		if runes < minCountedTokenRunes || shortStopwords[tok] {
			continue
		}
		total++
		switch {
		case runes >= minSubstringTokenRunes && strings.Contains(lowerMsg, tok):
			hits++
		case containsWord(words, tok):
			hits++
		case hasTypoOf(words, tok):
			hits += fuzzyHitWeight
		}
	}
	if total == 0 {
		return 0
	}
	return hits / float64(total)
}

func containsWord(words []string, tok string) bool {
	for _, w := range words {
		if w == tok {
			return true
		}
	}
	return false
}

func hasTypoOf(words []string, tok string) bool {
	t := []rune(tok)
	for _, w := range words {
		r := []rune(w)
		if len(r) < minCountedTokenRunes || r[0] != t[0] || max(len(r), len(t)) < minTypoLongerRunes {
			continue
		}
		if osaDistance(r, t) <= maxTypoDistance {
			return true
		}
	}
	return false
}

func osaDistance(a, b []rune) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}
