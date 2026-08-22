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

// guessNamesOwnAccount vive en flow (movement_text.go); acá queda solo el
// puente que usa buildCreateSeed.
func guessNamesOwnAccount(guess, description string) bool {
	return flow.GuessNamesOwnAccount(guess, description)
}

// mode discriminates buildCreateSeed's flow: a fresh CREATE vs a resolved
// UPDATE reusing the create flow. Stored under conversation.KeyMode.
// Las consts viven en flow (movement_write.go), acá el alias local.
const (
	modeCreate = flow.ModeCreate
	modeUpdate = flow.ModeUpdate
)

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

// accountNamedInMessage resuelve, del lado de la app, cuál de las cuentas del
// usuario nombra el MENSAJE — no el modelo. El account_id que manda el LLM en una
// fila que no es transfer se ignora: medido sobre llm_calls lo llenó en 30 de 43
// gastos, y en NINGUNO de esos 30 el usuario había nombrado una cuenta. Acertaba
// la default por el orden de los ids, no por entender el mensaje.
//
// Exige coverage 1.0 —TODOS los tokens del nombre presentes— y no "algún token".
// Con "algún token", "pago de monotributo" matchearía la cuenta "Mercado Pago"
// por la palabra "pago", y ese es el caso más común que hay.
//
// (0, false) significa "el mensaje no nombra ninguna": la fila sigue al guess y,
// si tampoco, cae en la default de su moneda dentro de movement.Normalize.
// (0, true) es "nombra más de una": que pregunte.
func accountNamedInMessage(msg string, accounts []account.Account, cur string) (found uint64, ambiguous bool) {
	haystack := foldAccents(strings.ToLower(msg))
	if haystack == "" {
		return 0, false
	}
	for _, a := range accounts {
		// La moneda tiene que coincidir: dos cuentas pueden llamarse igual en ARS
		// y USD, y meter el gasto en la otra rompe los dos saldos.
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

// guessBackedByMessage exige que el account_name_guess esté RESPALDADO por lo que
// el usuario escribió. El modelo copia líneas del bloque CUENTAS DEL USUARIO del
// prompt —se lo vio mandar "Banco Galicia (ARS)", con el paréntesis de la moneda
// pegado— y sin esta compuerta ese invento abre un gap: le pregunta al usuario a
// qué cuenta va un gasto donde nunca mencionó ninguna.
func guessBackedByMessage(guess, msg string) bool {
	if guess == "" {
		return false
	}
	return flow.TokenCoverage(guess, foldAccents(strings.ToLower(msg))) >= fullCoverage
}

// fullCoverage es "todos los tokens del nombre aparecen en el mensaje". Se usa en
// las dos compuertas de arriba, que tienen que exigir lo mismo.
const fullCoverage = 1.0

func buildCreateSeed(result orchestrator.CreateResult, taxonomy []orchestrator.TaxonomyEntry, accounts []account.Account, userText string) conversation.Data {
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
		// El account_id del modelo SÓLO vale en las piernas de un transfer. En un
		// gasto es ruido; ahí la cuenta la resuelve la app contra el mensaje —
		// la medición que lo decidió está en accountNamedInMessage.
		if draft.AccountID != nil && draft.Type == constants.Transfer {
			row.AccountID = strconv.FormatUint(*draft.AccountID, 10)
		}

		idx := strconv.Itoa(i)
		if draft.Category == constants.PendingReview || (len(known) > 0 && !known[draft.Category+"\x00"+draft.Subcategory]) {
			categoryGaps = append(categoryGaps, idx)
		}

		// Un gap de cuenta significa "no puedo saber a qué cuenta va esta fila y
		// tengo que preguntar". Una fila sin nada que resolver NO es un gap: es el
		// camino normal "usá mi default" y tiene que seguir siendo mudo.
		if draft.Type == constants.Transfer {
			// Una pierna sin resolver no se puede adivinar: un transfer necesita
			// las DOS cuentas o no es un transfer. Camino sin cambios.
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
				// El mensaje nombra más de una cuenta suya. Elegir sería tirar una
				// moneda sobre un saldo.
				accountGaps = append(accountGaps, idx)
			case named != 0:
				row.AccountID = strconv.FormatUint(named, 10)
			case guessBackedByMessage(draft.AccountNameGuess, userText) &&
				guessNamesOwnAccount(draft.AccountNameGuess, draft.Description):
				// El usuario nombró algo que no es ninguna de sus cuentas: hay que
				// preguntar, y ofrecerle crearla ("pagué el curso con Brubank").
				// Si fuera una cuenta EXISTENTE, accountNamedInMessage ya la habría
				// encontrado — por eso acá no se vuelve a llamar matchNamedAccount.
				accountGaps = append(accountGaps, idx)
			}
			// Sin ninguna de las tres: AccountID queda vacío y movement.Normalize
			// cae en la default de la moneda. Ese camino tiene que seguir mudo.
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
