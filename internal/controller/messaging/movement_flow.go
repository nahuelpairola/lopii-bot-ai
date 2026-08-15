package messaging

import (
	"strconv"
	"strings"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/constants"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

// guessNamesOwnAccount discrimina los dos motivos por los que el modelo llena
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
// Limitación heredada, no introducida: tokenAppearsInString exige tokens de
// minMatchTokenLen (4), así que un nombre de 3 letras ("Ana") no matchea y abre
// un gap de más. Es estrictamente más raro que la falla que reemplaza.
//
// Las dos direcciones las fijan TestResolveAndInsert_ExpenseNeverCreatesCounterpartyAccount
// y TestResolveAndInsert_NonTransferCreatesNamedOwnAccount.
func guessNamesOwnAccount(guess, description string) bool {
	if guess == "" {
		return false
	}
	return !tokenAppearsInString(guess, foldAccents(strings.ToLower(description)))
}

// accountPendingCreate is the sentinel MovementRow.AccountID value
// meaning "the user chose, mid-flow, to create this account" — real
// account ids are always numeric strings, so this can never collide.
const accountPendingCreate = "PENDING_CREATE"

// mode discriminates buildCreateSeed's flow: a fresh CREATE vs a resolved
// UPDATE reusing the create flow. Stored under conversation.KeyMode.
const (
	modeCreate = "create"
	modeUpdate = "update"
)

// buildCreateSeed converts a Call 2 CREATE (or a resolved Call 2
// UPDATE) result into the seed Data for the movement_create flow: one
// movement.MovementRow per draft, plus a queue of row indices whose
// category/subcategory landed in PENDING_REVIEW and a queue of row
// indices whose transfer referenced an account the LLM couldn't
// resolve to an existing one. mode is "create" or "update";
// oldTransactionID is only meaningful for "update" (see
// movement_update_flow.go builds its own seed (buildUpdateSeed) since an
// UPDATE's shape differs — before/after movements, no gap-filling in
// this feature's scope — rather than reusing this function.
// matchNamedAccount resuelve el nombre de cuenta que dijo el usuario contra sus
// cuentas reales. Devuelve 0 si no hay UNA sola coincidencia exacta.
//
// La comparación es exacta (plegando acentos y mayúsculas), nunca parcial: acá
// una coincidencia errada escribe el movimiento en la cuenta EQUIVOCADA, que es
// el único lugar del código donde un error chico es un bug contable. "Galicia"
// contra "banco galicia" NO matchea a propósito — cae al gap y pregunta, que es
// el comportamiento de hoy.
//
// Sin esto, nombrar una cuenta que existe abría el picker igual: medido en vivo
// el 2026-08-12 con "Lote cemento 45000", donde el modelo mandó
// account_name_guess="Mercado Pago" —la cuenta default del usuario, que existe—
// y el bot preguntó a cuál iba.
func matchNamedAccount(guess string, accounts []account.Account, cur string) uint64 {
	needle := foldAccents(strings.ToLower(strings.TrimSpace(guess)))
	if needle == "" {
		return 0
	}
	var found uint64
	for _, a := range accounts {
		// La moneda tiene que coincidir: dos cuentas pueden llamarse igual en ARS
		// y USD, y meter el gasto en la otra rompe los dos saldos.
		if cur != "" && a.Currency.String() != cur {
			continue
		}
		if foldAccents(strings.ToLower(a.Name)) != needle {
			continue
		}
		if found != 0 {
			return 0 // ambiguo: que pregunte
		}
		found = uint64(a.ID)
	}
	return found
}

func buildCreateSeed(result orchestrator.CreateResult, taxonomy []orchestrator.TaxonomyEntry, accounts []account.Account) conversation.Data {
	rows := make([]movement.MovementRow, 0, len(result.Movements))
	var categoryGaps, accountGaps []string

	// known indexa los pares (categoría, subcategoría) que el usuario realmente
	// tiene. El modelo debería devolver PENDING_REVIEW cuando duda, pero a veces
	// inventa un par que no existe; sin este set eso no marcaba gap, el flujo
	// insertaba derecho y FindByCategoryAndSubcategory fallaba — el movimiento se
	// perdía con un error genérico (causa de los create_failed en intent_events).
	// Taxonomía vacía = no validar: sin con qué comparar, no se inventan gaps.
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
			PaymentMethod:    draft.PaymentMethod,
			Description:      draft.Description,
			Date:             draft.Date,
			Group:            draft.Group,
		}
		if draft.AccountID != nil {
			row.AccountID = strconv.FormatUint(*draft.AccountID, 10)
		}

		idx := strconv.Itoa(i)
		if draft.Category == constants.PendingReview || (len(known) > 0 && !known[draft.Category+"\x00"+draft.Subcategory]) {
			categoryGaps = append(categoryGaps, idx)
		}
		// Un gap de cuenta significa "no puedo saber a qué cuenta va esta fila y
		// tengo que preguntar". Lo abren dos casos: una transferencia con una pata
		// sin resolver, y cualquier fila que nombró una cuenta que el modelo no pudo
		// matchear con una existente. Sin el segundo caso esa fila cae callada en la
		// cuenta default de la moneda y la cuenta nombrada nunca se crea — la
		// intención declarada por el usuario se descarta.
		//
		// Una fila sin AccountNameGuess y sin AccountID NO es un gap: es el camino
		// normal "usá mi default" y tiene que seguir siendo mudo. Tampoco lo es una
		// que solo repite algo que ya está en la description — ver guessNamesOwnAccount.
		if draft.AccountID == nil && (draft.Type == constants.Transfer || guessNamesOwnAccount(draft.AccountNameGuess, draft.Description)) {
			// Antes de preguntar: si el nombre que dijo el usuario ES una de sus
			// cuentas, ya está resuelto. Preguntarle a cuál va después de que la
			// nombró es hacerle repetir lo que acaba de decir.
			if id := matchNamedAccount(draft.AccountNameGuess, accounts, draft.Currency); id != 0 {
				row.AccountID = strconv.FormatUint(id, 10)
			} else {
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

// categoryGapsFor devuelve los índices de las filas cuyo par
// (categoría, subcategoría) no existe en la taxonomía del usuario, o quedó en
// PENDING_REVIEW. Es la misma prueba que hace buildCreateSeed, extraída para que
// UPDATE la use también: tenerla sólo en CREATE fue un bug vivo en el que una
// corrección que nombraba una categoría inexistente perdía el movimiento.
//
// Taxonomía vacía = no validar: sin con qué comparar, no se inventan gaps.
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

// resolveTaxonomyPair busca el par (categoría, subcategoría) que nombra un
// texto suelto del usuario: "proyecto hogar", "Vivienda | Proyecto hogar",
// "vivienda/proyecto hogar".
//
// Existe porque el usuario nombra UNA cosa y la taxonomía guarda DOS. Cuando
// dice "moveme esto a proyecto hogar" no está eligiendo una categoría, está
// eligiendo un par — y sin resolverlo la fila queda con la categoría escrita a
// mano y la subcategoría vacía, o sea un gap, o sea el picker: al usuario le
// preguntan lo que acaba de decir.
//
// Sólo resuelve lo INEQUÍVOCO. Con cero o más de una coincidencia devuelve
// false y el gap-fill se encarga, que es la degradación correcta: preguntar es
// caro, adivinar mal es un dato corrupto.
func resolveTaxonomyPair(text string, taxonomy []orchestrator.TaxonomyEntry) (category, subcategory string, ok bool) {
	needle := normalizeForTaxonomy(text)
	if needle == "" {
		return "", "", false
	}

	var hits []orchestrator.TaxonomyEntry
	for _, t := range taxonomy {
		// Las tres formas de nombrar el mismo par. El separador se normaliza a un
		// espacio antes, así que "Vivienda | Proyecto hogar" y "vivienda/proyecto
		// hogar" llegan acá idénticos.
		if normalizeForTaxonomy(t.Subcategory) == needle ||
			normalizeForTaxonomy(t.Category+" "+t.Subcategory) == needle {
			hits = append(hits, t)
		}
	}
	if len(hits) != 1 {
		return "", "", false
	}
	return hits[0].Category, hits[0].Subcategory, true
}

// normalizeForTaxonomy deja un nombre comparable: sin acentos, en minúsculas,
// con los separadores y los espacios de más colapsados a uno.
func normalizeForTaxonomy(s string) string {
	s = strings.ToLower(foldAccents(s))
	for _, sep := range []string{"|", "/", ">", "-", ":"} {
		s = strings.ReplaceAll(s, sep, " ")
	}
	return strings.Join(strings.Fields(s), " ")
}

// parseUintSlice turns the string-encoded movement IDs carried through
// conversation.Data back into real uint IDs, for repository calls that
// take []uint (SoftDeleteByIDs, ReplaceMovements).
func parseUintSlice(ids []string) ([]uint, error) {
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

// accountGapsFor devuelve los índices de las filas que NOMBRAN una cuenta que no
// se pudo resolver a una real.
//
// Es el gemelo de categoryGapsFor, y faltaba: en el camino de corrección
// `conversation.KeyPendingAccountGaps` iba hardcodeado en nil — el mismo bug que tenía la
// categoría. Sin gap, una fila con el nombre de la cuenta y sin id sale igual, y
// la escritura la manda a la cuenta POR DEFAULT de su moneda: el movimiento
// termina en otra cuenta que la que pidió el usuario, sin que nada avise.
//
// Una fila SIN nombre y sin id no es un gap: es el camino normal "usá mi
// default", y tiene que seguir siendo mudo.
func accountGapsFor(rows []movement.MovementRow) []string {
	var gaps []string
	for i, r := range rows {
		if r.AccountID == "" && r.AccountNameGuess != "" {
			gaps = append(gaps, strconv.Itoa(i))
		}
	}
	return gaps
}
