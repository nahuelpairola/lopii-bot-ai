package flow

import (
	"strconv"
	"strings"
)

// accentFolder strips the Spanish diacritics so a word typed without them still
// matches text stored with them ("panaderia" vs "panadería"). Seven runes cover
// the whole language, which is why this is a replacer and not
// golang.org/x/text/unicode/norm — that package is an indirect dependency today
// and promoting it to direct would be a lot of machinery for seven characters.
//
// ñ folds to n on purpose: this is only ever used for substring matching inside
// a message, never for storage or display, so "nino" finding "niño" is a feature.
var accentFolder = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
)

// FoldAccents plega los acentos del español ("panadería" -> "panaderia").
func FoldAccents(s string) string { return accentFolder.Replace(s) }

const minMatchTokenLen = 4

// TokenAppearsInString reports whether any whitespace-separated token of
// `field` with length >= minMatchTokenLen is a substring of the
// already-lowercased haystack.
func TokenAppearsInString(field, lowerHaystack string) bool {
	for _, tok := range strings.Fields(FoldAccents(strings.ToLower(field))) {
		// ponytail: length>=4 skips es stopwords (de/en/el/con/por) without a
		// stopword list; standalone <=3-char descriptions like "pan"/"ypf"
		// won't match as tokens — revisit if that bites.
		if len([]rune(tok)) >= minMatchTokenLen {
			if strings.Contains(lowerHaystack, tok) {
				return true
			}
		}
	}
	return false
}

// GuessNamesOwnAccount discrimina los dos motivos por los que el modelo llena
// AccountNameGuess en una fila que NO es transferencia:
//
//   - "pagué el curso con Brubank" → Brubank es una cuenta del usuario que
//     todavía no existe. Hay que preguntar y, si él lo pide, crearla.
//   - "pizza con Pablo" → Pablo es la contraparte, no una cuenta. Crear una
//     cuenta "Pablo" sería un error, y hasta preguntar sería interrumpir un
//     gasto que hoy se guarda solo.
//
// La señal es DÓNDE cae el nombre. Si es parte de lo que pasó, queda en la
// description ("pizza con Pablo") y no es una cuenta. Si la description es la
// cosa comprada y el nombre quedó afuera ("pagué el curso con Brubank"), es una
// cuenta.
//
// Antes esto se decidía comparando el guess contra merchant, que ya no existe.
// Sin reemplazo la condición colapsaba a `guess != ""` y CADA contraparte
// abriría un gap ofreciendo crear una cuenta con el nombre de una persona.
//
// Limitación heredada, no introducida: TokenAppearsInString exige tokens de
// minMatchTokenLen (4), así que un nombre de 3 letras ("Ana") no matchea y abre
// un gap de más. Es estrictamente más raro que la falla que reemplaza.
//
// Las dos direcciones las fijan TestResolveAndInsert_ExpenseNeverCreatesCounterpartyAccount
// y TestResolveAndInsert_NonTransferCreatesNamedOwnAccount.
func GuessNamesOwnAccount(guess, description string) bool {
	if guess == "" {
		return false
	}
	return !TokenAppearsInString(guess, FoldAccents(strings.ToLower(description)))
}

// ParseUintSlice turns the string-encoded movement IDs carried through
// conversation.Data back into real uint IDs, for repository calls that
// take []uint (SoftDeleteByIDs, ReplaceMovements).
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
