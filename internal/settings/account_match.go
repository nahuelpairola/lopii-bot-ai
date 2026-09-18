package settings

import (
	"regexp"
	"strings"
	"unicode"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/flow"
)

var usdWords = map[string]bool{"usd": true, "dolar": true, "dolares": true, "verdes": true}

var usdSymbols = []string{"u$s", "us$"}

func words(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.FieldsFunc(flow.FoldAccents(strings.ToLower(s)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		set[w] = true
	}
	return set
}

func namesAccount(name string, msgWords map[string]bool) bool {
	nameWords := words(name)
	if len(nameWords) == 0 {
		return false
	}
	for w := range nameWords {
		if !msgWords[w] {
			return false
		}
	}
	return true
}

func currencyCue(msg string, msgWords map[string]bool) currency.Currency {
	lower := strings.ToLower(msg)
	for _, sym := range usdSymbols {
		if strings.Contains(lower, sym) {
			return currency.USD
		}
	}
	for w := range usdWords {
		if msgWords[w] {
			return currency.USD
		}
	}
	return currency.ARS
}

func accountNamedInText(msg string, accs []account.Account) (uint64, bool) {
	msgWords := words(msg)
	var named []account.Account
	for _, a := range accs {
		if namesAccount(a.Name, msgWords) {
			named = append(named, a)
		}
	}
	if len(named) == 1 {
		return uint64(named[0].ID), true
	}
	cur := currencyCue(msg, msgWords)
	var found []account.Account
	for _, a := range named {
		if a.Currency == cur {
			found = append(found, a)
		}
	}
	if len(found) != 1 {
		return 0, false
	}
	return uint64(found[0].ID), true
}

var amountShapes = []*regexp.Regexp{
	regexp.MustCompile(`^\d+$`),
	regexp.MustCompile(`^\d+,\d+$`),
	regexp.MustCompile(`^\d{1,3}(\.\d{3})+(,\d+)?$`),
	regexp.MustCompile(`^\d+\.\d{1,2}$`),
}

var amountMultipliers = map[string]bool{
	"mil": true, "k": true, "luca": true, "lucas": true, "palo": true, "palos": true,
	"m": true, "millon": true, "millones": true,
}

var adjustStems = []string{"ajust", "saldo", "balance", "monto", "correg", "actualiz", "valor", "tengo"}

var otherOperationStems = []string{
	"nombre", "renombr", "llam", "defecto", "default", "predetermin", "principal",
	"nueva", "nuevo", "crea", "elimin", "borr",
}

func isAmountShape(tok string) bool {
	for _, re := range amountShapes {
		if re.MatchString(tok) {
			return true
		}
	}
	return false
}

func trimAmountToken(tok string) string {
	for _, sym := range append(usdSymbols, "$") {
		tok = strings.TrimPrefix(tok, sym)
	}
	return strings.TrimRight(tok, ".,;:!?)")
}

func statedTotal(msg string, accs []account.Account) (total string, stated bool) {
	nameWords := map[string]bool{}
	for _, a := range accs {
		for w := range words(a.Name) {
			nameWords[w] = true
		}
	}
	toks := strings.Fields(flow.FoldAccents(strings.ToLower(msg)))
	found := ""
	for i, raw := range toks {
		tok := trimAmountToken(raw)
		if !strings.ContainsAny(tok, "0123456789") || strings.ContainsAny(tok, "/:") || nameWords[tok] {
			continue
		}
		if !isAmountShape(tok) {
			return "", true
		}
		if i+1 < len(toks) && amountMultipliers[strings.Trim(toks[i+1], ".,;:!?")] {
			return "", true
		}
		if found != "" && found != tok {
			return "", true
		}
		found = tok
	}
	return found, found != ""
}

func hasStem(msgWords map[string]bool, stems []string) bool {
	for w := range msgWords {
		for _, st := range stems {
			if strings.HasPrefix(w, st) {
				return true
			}
		}
	}
	return false
}

func adjustIntent(msg string, accs []account.Account, accountResolved bool) (adjust bool, total string) {
	msgWords := words(msg)
	if hasStem(msgWords, otherOperationStems) {
		return false, ""
	}
	total, stated := statedTotal(msg, accs)
	if hasStem(msgWords, adjustStems) || (stated && accountResolved) {
		return true, total
	}
	return false, ""
}
