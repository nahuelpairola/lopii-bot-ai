package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messages"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/trace"
)

func movementToRow(m movement.Movement) movement.MovementRow {
	row := movement.MovementRow{
		Type:     string(m.Type),
		Amount:   messages.DisplayAmount(m.Amount),
		Currency: m.Currency.String(),
		Date:     m.Date.Format("2006-01-02"),
	}
	if m.Subcategory != nil {
		row.Category = m.Subcategory.Category
		row.Subcategory = m.Subcategory.Subcategory
		row.Icon = m.Subcategory.Icon
	}
	if m.AccountID != nil {
		row.AccountID = strconv.FormatUint(*m.AccountID, 10)
	}
	// Una corrección reescribe el grupo ENTERO (ReplaceMovements), así que las
	// dos cosas que hacen grupo a un grupo tienen que viajar en la fila: el tag
	// que vuelve a unir las patas y cuál de las dos es la que sale. Sin el tag
	// las patas se insertan sueltas; sin la marca vuelven las dos en positivo.
	// Las dos las rechaza validateTransferGroups y la corrección muere entera.
	if m.Type == movement.Transfer && m.TransactionID != nil {
		row.Group = m.TransactionID.String()
		if m.Amount.IsNegative() {
			row.TransferOut = movement.TransferOutMark
		}
	}
	if m.Account != nil {
		row.AccountName = m.Account.Name
	}
	if m.PaymentMethod != nil {
		row.PaymentMethod = *m.PaymentMethod
	}
	if m.Description != nil {
		row.Description = *m.Description
	}
	return row
}

func rowToDraft(r movement.MovementRow) orchestrator.MovementDraft {
	draft := orchestrator.MovementDraft{
		Type:             r.Type,
		Amount:           r.Amount,
		Currency:         r.Currency,
		AccountNameGuess: r.AccountNameGuess,
		Category:         r.Category,
		Subcategory:      r.Subcategory,
		PaymentMethod:    r.PaymentMethod,
		Description:      r.Description,
		Date:             r.Date,
		Group:            r.Group,
	}
	if r.AccountID != "" && r.AccountID != flow.AccountPendingCreate {
		if id, err := strconv.ParseUint(r.AccountID, 10, 64); err == nil {
			draft.AccountID = &id
		}
	}
	return draft
}

func draftToRow(d orchestrator.MovementDraft) movement.MovementRow {
	row := movement.MovementRow{
		Type:             d.Type,
		Amount:           d.Amount,
		Currency:         d.Currency,
		AccountNameGuess: d.AccountNameGuess,
		Category:         d.Category,
		Subcategory:      d.Subcategory,
		PaymentMethod:    d.PaymentMethod,
		Description:      d.Description,
		Date:             d.Date,
		Group:            d.Group,
	}
	if d.AccountID != nil {
		row.AccountID = strconv.FormatUint(*d.AccountID, 10)
	}
	return row
}

// EncodeCandidateGroups convierte los transactionGroup recién buscados a su
// forma de Data basada en filas, para que el picker de candidato ambiguo pueda
// llevar el estado "before" completo sin una segunda vuelta a la DB. Exportada
// porque los tests de borde la usan.
func EncodeCandidateGroups(groups []transactionGroup) []interface{} {
	converted := make([]flow.CandidateGroup, 0, len(groups))
	for _, g := range groups {
		converted = append(converted, toCandidateGroup(g))
	}
	return encodeCandidateGroupList(converted)
}

// encodeCandidateGroupList existe aparte porque el drenaje del agent loop ya
// tiene flow.CandidateGroup (viene del payload parkeado) y nunca tuvo el
// transactionGroup con los movimientos enteros. El formato vive en flow
// (flow.EncodeCandidateGroups); acá solo se delega.
func encodeCandidateGroupList(groups []flow.CandidateGroup) []interface{} {
	return flow.EncodeCandidateGroups(groups)
}

// ChangeAsk es en qué punto está la pregunta de "qué cambiarle al movimiento".
// Los dos estados no son excluyentes en el tipo pero sí en la vida: primero se
// toca un botón (pickedField), después se escribe el valor (gaveValue).
// Exportado porque el drenaje de borde (job_drain) lo usa como valor cero.
type ChangeAsk struct {
	// pickedField: tocó uno de los botones, o sea nombró el CAMPO. Falta el valor.
	pickedField bool
	// gaveValue: escribió algo como valor nuevo. Si con eso tampoco sale una
	// corrección, no hay más que preguntar.
	gaveValue bool
	// answer es lo que contestó, SIN el texto original pegado adelante. Es lo
	// único que se puede parsear.
	answer string
	// field es CUÁL campo eligió con el botón. Tiene que sobrevivir a la segunda
	// vuelta de la pregunta: sin él, al llegar el valor la app sabe que eligió
	// algo pero no qué, y la corrección se le vuelve a caer al modelo.
	field string
}

// amountOnlyCorrection arma la corrección del lado de la app cuando lo único
// que hay que cambiar es el monto y el usuario ya lo escribió.
//
// Las cuatro condiciones son todas necesarias:
//   - contestó por texto (gaveValue): si tocó un botón, el campo NO es el monto
//   - no tocó ningún botón antes (pickedField): si eligió "La fecha", un número
//     suelto sería un día, no un monto
//   - una sola fila: en una transferencia de dos piernas "el monto" es ambiguo
//   - parsea como monto positivo: un 0 significa borrar, y esa semántica
//     ("regalo/gratis") la resuelve mejor el camino de siempre
func amountOnlyCorrection(before []movement.MovementRow, ask ChangeAsk) ([]orchestrator.MovementDraft, bool) {
	if !ask.gaveValue || ask.pickedField || len(before) != 1 {
		return nil, false
	}
	amt, err := movement.ParseARAmount(ask.answer)
	if err != nil || !amt.IsPositive() {
		return nil, false
	}
	after := before[0]
	after.Amount = amt.String()
	return []orchestrator.MovementDraft{rowToDraft(after)}, true
}

// proceedToUpdateConfirm runs Call 2 UPDATE against a candidate found
// via resolveCandidates — both the single-match path and the
// post-picker path funnel through here.
// ask lleva en qué punto está la pregunta de "qué cambiar": si nunca se
// preguntó, si el usuario tocó un botón (nombró el campo, falta el valor) o si
// ya intentó decir el valor. Sólo el drenaje la manda con algo adentro.
func proceedToUpdateConfirm(ctx context.Context, svc agentServices, b *bot.Bot, chatID int64, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, ask ChangeAsk) error {
	// Si lo único que se sumó al pedido fue el NOMBRE del campo ("La
	// categoría"), no hay ningún valor que resolver todavía. Preguntarlo antes
	// de llamar al modelo ahorra la llamada entera — ~1.100 tokens que iban a
	// volver sin cambiar nada.
	if ask.pickedField && !ask.gaveValue {
		return parkChangeQuestion(ctx, svc, b, chatID, userID, message, transactionID, oldIDs, beforeRows, ask)
	}

	// Atajo del monto. La pregunta fue "¿Cuánto era?" y contestó un número: no
	// queda NADA que interpretar, y movement.ParseARAmount ya lo sabe leer. Mandárselo al
	// modelo cuesta ~1.500 tokens para que copie el número — y le da la
	// oportunidad de tocar de paso algo que nadie le pidió.
	//
	// El gate de confirmación NO se saltea: sigue pasando por
	// seedAndStartUpdateConfirm, así que el usuario ve el antes/después igual.
	if after, ok := amountOnlyCorrection(beforeRows, ask); ok {
		return seedAndStartUpdateConfirm(ctx, svc, b, chatID, userID, message, oldIDs, beforeRows,
			orchestrator.UpdateResult{Resolved: true, Movements: after})
	}

	drafts := make([]orchestrator.MovementDraft, 0, len(beforeRows))
	for _, row := range beforeRows {
		drafts = append(drafts, rowToDraft(row))
	}

	accs, err := svc.FindUserAccounts(userID)
	if err != nil {
		return err
	}
	accountOptions := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		accountOptions = append(accountOptions, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}

	result, err := svc.ResolveUpdate(ctx, message, orchestrator.MovementCandidate{
		TransactionID: transactionID,
		Movements:     drafts,
	}, accountOptions)
	if err != nil {
		return err
	}
	// Dos formas de no entender el CAMBIO, y hay que atrapar las dos.
	//
	// La declarada (!Resolved) es la que el modelo admite. La otra la vimos en
	// producción con "el café estaba mal" (traza 317df846): devolvió
	// Resolved=true y el movimiento IDÉNTICO, o sea inventó una corrección que
	// no corrige nada. Confirmarla haría un DELETE+INSERT para dejar todo igual
	// —quemando un id y contando como update_confirmed— y al usuario le
	// mostraría "$1.800 (antes: $1.800)".
	//
	// Por eso el chequeo no puede depender de que el modelo se declare incapaz:
	// una corrección que no cambia nada no es una corrección.
	if !result.Resolved || correctionIsNoOp(beforeRows, result.Movements) {
		if ask.gaveValue {
			// Ya nos dijo el valor por texto y seguimos sin entender: cortar es
			// más honesto que volver a preguntar lo mismo.
			resolveMetric(ctx, svc, userID, outcomeLoopDidNothing)
			svc.SendText(ctx, b, chatID, messages.MsgStillCannotCorrect)
			return nil
		}
		// El candidato ya está resuelto acá: lo que falló es entender el CAMBIO.
		// Antes esto era un callejón sin salida —"no me quedó claro, decímelo de
		// nuevo"— y el usuario que había nombrado bien el movimiento se quedaba
		// sin nada. Ahora se le pregunta, que es la máquina de preguntas que la
		// etapa 2 ya construyó.
		return parkChangeQuestion(ctx, svc, b, chatID, userID, message, transactionID, oldIDs, beforeRows, ask)
	}

	return seedAndStartUpdateConfirm(ctx, svc, b, chatID, userID, message, oldIDs, beforeRows, result)
}

// correctionIsNoOp dice si la corrección "resuelta" deja el movimiento igual
// que como estaba. Se compara campo por campo y no con reflect.DeepEqual porque
// los dos lados vienen de fuentes distintas: el antes sale de la DB (montos ya
// formateados, cuenta resuelta) y el después del modelo, que omite lo que no
// toca. Un DeepEqual daría "cambió" siempre.
func correctionIsNoOp(before []movement.MovementRow, after []orchestrator.MovementDraft) bool {
	if len(before) == 0 || len(before) != len(after) {
		return false // otra cantidad de filas ES un cambio (o no hay con qué comparar)
	}
	for i, draft := range after {
		if !sameMovementForCorrection(before[i], draftToRow(draft)) {
			return false
		}
	}
	return true
}

// sameMovementForCorrection compara los campos que una corrección puede tocar.
//
// Un campo vacío del lado nuevo se lee como "no lo tocó", no como "lo borró":
// el modelo devuelve sólo lo que cambia, y tratarlo como borrado haría ver
// cambios donde no los hay — que es el error que dejaría pasar el no-op.
func sameMovementForCorrection(before, after movement.MovementRow) bool {
	unchanged := func(b, a string) bool { return a == "" || a == b }
	return sameAmount(before.Amount, after.Amount) &&
		unchanged(before.Type, after.Type) &&
		unchanged(before.Currency, after.Currency) &&
		unchanged(before.Category, after.Category) &&
		unchanged(before.Subcategory, after.Subcategory) &&
		unchanged(before.Date, after.Date) &&
		unchanged(before.AccountID, after.AccountID) &&
		// AccountNameGuess, no sólo AccountID: cambiar de cuenta pone el NOMBRE y
		// VACÍA el id, y `unchanged` trata el vacío como "no lo tocó". Sin esta
		// línea, "el peaje ponelo en banco galicia" se veía idéntico al original y
		// la guarda de no-op se lo tragaba con un "eso ya estaba así".
		unchanged(before.AccountNameGuess, after.AccountNameGuess) &&
		unchanged(before.Description, after.Description)
}

// sameAmount compara montos por VALOR, no por texto: "1800" y "1800.00" son el
// mismo monto y la comparación de strings diría que cambió.
func sameAmount(before, after string) bool {
	if after == "" {
		return true // no lo tocó
	}
	b, berr := movement.ParseARAmount(before)
	a, aerr := movement.ParseARAmount(after)
	if berr != nil || aerr != nil {
		return before == after
	}
	return b.Abs().Equal(a.Abs())
}

// parkChangeQuestion guarda el candidato YA resuelto y pregunta qué cambiarle.
// El candidato no se vuelve a buscar: encontrarlo fue la mitad cara, y volver a
// resolverlo con el texto nuevo ("2000") lo perdería — ese texto no nombra
// ningún movimiento.
func parkChangeQuestion(ctx context.Context, svc agentServices, b *bot.Bot, chatID int64, userID uint64, change, transactionID string, oldIDs []string, rows []movement.MovementRow, ask ChangeAsk) error {
	if !svc.ActionsEnabled() {
		// Sin cola no hay a dónde parkear: el camino viejo sigue siendo mejor
		// que quedarse mudo.
		resolveMetric(ctx, svc, userID, outcomeParkFailed)
		svc.SendText(ctx, b, chatID, flow.MsgSomethingBroke)
		return nil
	}
	payload, err := json.Marshal(agentPayload{
		Change:            change,
		Candidates:        []flow.CandidateGroup{{TransactionID: transactionID, OldIDs: oldIDs, Rows: rows}},
		Chosen:            0,
		PickedChangeField: ask.pickedField,
		PickedField:       ask.field,
	})
	if err != nil {
		return fmt.Errorf("park change question: payload: %w", err)
	}
	// Segunda vuelta: ya tocó el botón del campo, así que lo único que falta es
	// el valor — y ahí los botones sobran, cualquiera de ellos ya se usó.
	question := pendingaction.OpenQuestion{
		Key:     questionKeyChange,
		Prompt:  messages.MsgAskWhatToChange(rows),
		Options: changeFieldOptions(),
	}
	if ask.pickedField {
		question.Prompt, question.Options = messages.MsgAskChangeValue, nil
	}
	questions, err := json.Marshal([]pendingaction.OpenQuestion{question})
	if err != nil {
		return fmt.Errorf("park change question: questions: %w", err)
	}
	row := &pendingaction.PendingAction{
		UserID: userID, Tool: orchestrator.ToolCorrectMovement,
		Payload: payload, Questions: questions,
		Budget: 1 + budgetSlack, TraceID: trace.ID(ctx),
	}
	if err := svc.ActionsInsert(row); err != nil {
		return fmt.Errorf("park change question: %w", err)
	}
	return drainNextAgentAction(ctx, svc, b, chatID, userID)
}

// seedAndStartUpdateConfirm builds the confirm flow's seed from an
// already-resolved UpdateResult (never calls the orchestrator itself)
// and starts it. Called by proceedToUpdateConfirm once Call 2 UPDATE
// resolves.
// userTaxonomy carga los pares de la taxonomía del usuario. Un error devuelve
// nil a propósito: sin con qué comparar no se inventan gaps.
func userTaxonomy(svc agentServices, userID uint64) []orchestrator.TaxonomyEntry {
	subs, err := svc.SubcategoriesFindAllForUser(userID)
	if err != nil {
		return nil
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, s := range subs {
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: s.Category, Subcategory: s.Subcategory})
	}
	return taxonomy
}

// applyStructuredCorrection aplica una corrección que ya viene en campos, SIN
// volver a llamar al modelo.
//
// Es el camino corto y es el bueno: el modelo ya dijo qué cambiar cuando eligió
// la tool, así que una segunda llamada sólo le daría la oportunidad de tocar de
// paso algo que nadie le pidió — que es literalmente lo que pasó con "el café
// estaba mal" (traza 317df846). El gate de confirmación NO se saltea: el usuario
// ve el antes/después igual.
func applyStructuredCorrection(ctx context.Context, svc agentServices, b *bot.Bot, chatID int64, userID uint64, payload agentPayload, groups []flow.CandidateGroup) error {
	before := make([][]movement.MovementRow, 0, len(groups))
	var oldIDs []string
	for _, g := range groups {
		before = append(before, g.Rows)
		oldIDs = append(oldIDs, g.OldIDs...)
	}

	// Las guardas de conjunto necesitan los grupos separados y lo que el mensaje
	// nombró. NamedAccount sale de la app, no del modelo: es un dato, no una
	// interpretación.
	after, err := applyChangesToSet(before, payload.Changes, guardContext{
		Scope:        payload.Scope,
		NamedAccount: accountNamedIn(svc, userID, payload.Change),
		Message:      payload.Change,
	})
	if err != nil {
		// Un cambio imposible (un reintegro más grande que la compra, "poné todos
		// en 1500", un valor ilegible) se le dice al usuario. Aplicarlo a medias
		// sería peor: un lote donde algunos cambiaron y otros no, sin manera de
		// saber cuáles.
		slog.WarnContext(ctx, "structured correction rejected", "user_id", userID, "err", err)
		resolveMetric(ctx, svc, userID, outcomeLoopDidNothing)
		// Una contradicción NO es un "no te entendí": el bot entendió y se niega.
		// Decirle lo genérico lo manda a reformular algo que ya dijo bien.
		// Cada guarda que significa algo distinto se dice distinto. Las que quedan
		// en la genérica son fallas del MODELO (un op sobre un campo que no lo
		// acepta, un valor ilegible), no cosas que el usuario pueda arreglar
		// sabiendo cuál fue.
		if errors.Is(err, errRefundThatGrows) {
			svc.SendText(ctx, b, chatID, messages.MsgRefundWouldGrow)
			return nil
		}
		if errors.Is(err, errRefundExceedsAmount) {
			svc.SendText(ctx, b, chatID, messages.MsgRefundExceeds)
			return nil
		}
		if errors.Is(err, errAmbiguousSetAll) {
			svc.SendText(ctx, b, chatID, messages.MsgAmbiguousSetAll)
			return nil
		}
		svc.SendText(ctx, b, chatID, messages.MsgStillCannotCorrect)
		return nil
	}

	// El usuario nombra UNA cosa ("proyecto hogar") y la taxonomía guarda DOS.
	// applyChange deja la subcategoría vacía justamente para que el par se
	// resuelva acá, que es donde hay taxonomía. Lo que no se resuelve queda como
	// gap y lo pregunta el gap-fill.
	taxonomy := userTaxonomy(svc, userID)
	// Y las cuentas, para resolver la que el usuario nombró. Sin esto la fila sale
	// con el NOMBRE y sin id, y la escritura la manda a la cuenta por default de
	// su moneda: el movimiento termina en otra cuenta que la pedida, en silencio.
	accounts, _ := svc.FindUserAccounts(userID)
	beforeRows := make([]movement.MovementRow, 0, len(oldIDs))
	for _, g := range before {
		beforeRows = append(beforeRows, g...)
	}
	drafts := make([]orchestrator.MovementDraft, 0, len(beforeRows))
	for _, g := range after {
		for _, r := range g {
			if r.Subcategory == "" {
				if cat, sub, ok := resolveTaxonomyPair(r.Category, taxonomy); ok {
					r.Category, r.Subcategory = cat, sub
				}
			}
			// Misma idea con la cuenta: el usuario dice "banco galicia" y la app
			// lo resuelve a un id. Lo que no resuelve queda como gap y lo pregunta
			// el gap-fill — nunca cae mudo en la cuenta por default.
			if r.AccountID == "" && r.AccountNameGuess != "" {
				if id := matchNamedAccount(r.AccountNameGuess, accounts, r.Currency); id != 0 {
					r.AccountID = strconv.FormatUint(id, 10)
				}
			}
			drafts = append(drafts, rowToDraft(r))
		}
	}

	// Una corrección que deja todo igual NO es una corrección. Confirmarla haría
	// un DELETE+INSERT para no cambiar nada: quema un id, cuenta como
	// update_confirmed, y al usuario le muestra "$45.000 (antes: $45.000)".
	//
	// El camino viejo ya tenía esta guarda (correctionIsNoOp en
	// proceedToUpdateConfirm) y el estructurado nació sin ella — paridad de
	// migración otra vez. Se vio en vivo el 2026-08-12: contestar la categoría
	// que el movimiento YA tenía reemplazó la fila igual.
	if correctionIsNoOp(beforeRows, drafts) {
		resolveMetric(ctx, svc, userID, outcomeLoopDidNothing)
		svc.SendText(ctx, b, chatID, messages.MsgCorrectionChangesNothing)
		return nil
	}

	return seedAndStartUpdateConfirm(ctx, svc, b, chatID, userID, payload.Change, oldIDs, beforeRows,
		orchestrator.UpdateResult{Resolved: true, Movements: drafts})
}

// carryTransferIdentity devuelve las filas corregidas con la identidad de grupo
// del ANTES: el tag que vuelve a unir las patas y cuál es la que sale.
//
// Las dos las pone la app y ninguna sobrevive el viaje por MovementDraft — el
// modelo no las conoce y no tiene por qué. Sin esto una transferencia corregida
// se reinserta como dos patas sueltas en positivo y validateTransferGroups
// rechaza el grupo entero: la corrección muere SIEMPRE, cualquiera sea el campo.
//
// El apareo es posicional, que es lo mismo que ya asume correctionIsNoOp: el
// modelo devuelve las filas del candidato en el orden en que se las dieron. Con
// otra cantidad de filas no hay a qué aparear y se devuelve lo que había — el
// guard rechaza después, que es lo correcto: mejor ruidoso que mal firmado.
func carryTransferIdentity(before, after []movement.MovementRow) []movement.MovementRow {
	if len(before) != len(after) {
		return after
	}
	for i := range after {
		if before[i].Group != "" {
			after[i].Group = before[i].Group
		}
		after[i].TransferOut = before[i].TransferOut
	}
	return after
}

// accountNamedIn devuelve el nombre de la cuenta del usuario que aparece en el
// mensaje, o "" si no nombró ninguna. Es lo que separa un reintegro a la misma
// cuenta de uno que entró en otra — restar el segundo del gasto original deja
// DOS saldos mal.
func accountNamedIn(svc agentServices, userID uint64, message string) string {
	accs, err := svc.FindUserAccounts(userID)
	if err != nil {
		return ""
	}
	folded := foldAccents(strings.ToLower(message))
	for _, a := range accs {
		if strings.Contains(folded, foldAccents(strings.ToLower(a.Name))) {
			return a.Name
		}
	}
	return ""
}

func seedAndStartUpdateConfirm(ctx context.Context, svc agentServices, b *bot.Bot, chatID int64, userID uint64, userMessage string, oldIDs []string, beforeRows []movement.MovementRow, result orchestrator.UpdateResult) error {
	accs, _ := svc.FindUserAccounts(userID)
	nameByID := make(map[string]string, len(accs))
	for _, a := range accs {
		nameByID[strconv.FormatUint(uint64(a.ID), 10)] = a.Name
	}

	afterRows := make([]movement.MovementRow, 0, len(result.Movements))
	for _, d := range result.Movements {
		row := draftToRow(d)
		if sub, err := svc.FindSubcategory(userID, row.Category, row.Subcategory); err == nil {
			row.Icon = sub.Icon
		}
		if row.AccountID != "" {
			row.AccountName = nameByID[row.AccountID]
		}
		afterRows = append(afterRows, row)
	}
	afterRows = carryTransferIdentity(beforeRows, afterRows)

	// La taxonomía del usuario, para validar el par igual que CREATE. Si no
	// carga, taxonomy queda vacía y categoryGapsFor no inventa gaps — degradar a
	// "no valido" es correcto; degradar a "borro el movimiento" no lo era.
	taxonomy := userTaxonomy(svc, userID)

	// Paridad con CREATE, y era un bug VIVO: acá iba conversation.EncodeStringSlice(nil)
	// hardcodeado, así que una corrección que nombraba una categoría inexistente
	// no marcaba gap, el flujo insertaba derecho y FindByCategoryAndSubcategory
	// fallaba — el movimiento se perdía con un error genérico. Es exactamente lo
	// que el comentario de buildCreateSeed documenta que pasaba en CREATE antes de
	// tener el set `known`. Y updateSystemPromptTemplate no lleva taxonomía NI la
	// regla de "no inventes nombres", así que el camino de corrección emite pares
	// arbitrarios con total libertad.
	seed := conversation.Data{
		conversation.KeyMode:                modeUpdate,
		conversation.KeyOldMovementIDs:      conversation.EncodeStringSlice(oldIDs),
		conversation.KeyBeforeMovements:     movement.EncodeMovementRows(beforeRows),
		conversation.KeyMovements:           movement.EncodeMovementRows(afterRows),
		conversation.KeyPendingCategoryGaps: conversation.EncodeStringSlice(categoryGapsFor(afterRows, taxonomy)),
		conversation.KeyPendingAccountGaps:  conversation.EncodeStringSlice(accountGapsFor(afterRows)),
		conversation.KeyDeleteInstead:       strconv.FormatBool(correctionIsDeletion(afterRows, userMessage)),
	}

	// Con un gap de categoría el destino cambia: al flujo de gap-fill, que es el
	// que sabe preguntar y ofrecer "crear una categoría nueva". El seed ya viene
	// en modeUpdate, así que persistMovements hace ReplaceMovements y no un
	// insert — la maquinaria estaba entera, sólo que nadie la alcanzaba desde
	// acá.
	//
	// Se pierde el diff antes/después en ese caso, y es un intercambio a
	// conciencia: antes el movimiento se PERDÍA con un error genérico.
	flowName := flow.MovementUpdateConfirmFlowName
	if len(conversation.DecodeStringSlice(seed, conversation.KeyPendingCategoryGaps)) > 0 ||
		len(conversation.DecodeStringSlice(seed, conversation.KeyPendingAccountGaps)) > 0 {
		flowName = flow.MovementCreateFlowName
	}

	prompt, err := svc.EngineStartWithData(userID, flowName, seed)
	if err != nil {
		return err
	}
	svc.SendPrompt(ctx, b, chatID, prompt)
	return nil
}

// finishMovementUpdatePickFlow runs once the user has picked a
// candidate from an ambiguous list — it resolves the index back to the
// full candidate (both were seeded together) and hands off to
// proceedToUpdateConfirm.
func finishMovementUpdatePickFlow(ctx context.Context, svc agentServices, b *bot.Bot, chatID int64, data conversation.Data) {
	idx, err := strconv.Atoi(conversation.StringOrEmpty(data["chosen_index"]))
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: flow.MsgSomethingBroke})
		return
	}

	candidates := flow.DecodeCandidateGroups(data)
	if idx < 0 || idx >= len(candidates) {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: flow.MsgSomethingBroke})
		return
	}
	chosen := candidates[idx]

	message := conversation.StringOrEmpty(data["message"])
	if err := proceedToUpdateConfirm(ctx, svc, b, chatID, data.UserID(), message, chosen.TransactionID, chosen.OldIDs, chosen.Rows, ChangeAsk{}); err != nil {
		if svc.EnqueueUpdatePickIfRateLimited(ctx, b, chatID, data.UserID(), message, chosen.TransactionID, chosen.OldIDs, chosen.Rows, err) {
			return
		}
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: flow.MsgSomethingBroke})
	}
}

// palabrasDeMontoCero: las formas de decir "no salió nada" que no traen ningún
// dígito. Sin esto "me lo regalaron" no podría borrar nunca.
var palabrasDeMontoCero = []string{"gratis", "regal", "nada", "cero", "invit"}

// messageNamesAnAmount dice si el usuario habló de plata: un dígito, o una de
// las palabras que significan que no salió nada.
func messageNamesAnAmount(s string) bool {
	low := foldAccents(strings.ToLower(s))
	if strings.ContainsAny(low, "0123456789") {
		return true
	}
	for _, w := range palabrasDeMontoCero {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

// correctionIsDeletion reports whether an UPDATE's corrected set nullifies the
// movement entirely — every row's amount parses to zero. Per the chosen
// "regalo/gratis total" semantics a correction to 0 deletes the movement
// rather than storing an illegal amount-0 row (the guard rejects amount 0). A
// mixed set (some 0, some not) or an unparseable amount returns false and
// falls through to the guard.
//
// userMessage NO es decorativo. El 2026-08-10 "Editá los movimientos de lote
// de hoy" —que no dice ningún cambio— llegó acá con los montos en 0 y armó un
// BORRADO que el usuario confirmó; sólo no borró porque la escritura falló. Si
// el usuario no habló de plata, unos montos en 0 son un fallo del modelo y no
// una intención, y borrar seria destruir datos por una alucinación.
func correctionIsDeletion(rows []movement.MovementRow, userMessage string) bool {
	if len(rows) == 0 || !messageNamesAnAmount(userMessage) {
		return false
	}
	for _, r := range rows {
		amt, err := movement.ParseARAmount(r.Amount)
		if err != nil || !amt.IsZero() {
			return false
		}
	}
	return true
}
