package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/trace"
)

const msgStillCannotCorrect = "Sigo sin darme cuenta qué cambiarle. Probá diciéndomelo derecho — ej: «el café fueron 2000»."

func movementToRow(m movement.Movement) movement.MovementRow {
	row := movement.MovementRow{
		Type:     string(m.Type),
		Amount:   m.Amount.Abs().String(),
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

func EncodeCandidateGroups(groups []transactionGroup) []interface{} {
	converted := make([]flow.CandidateGroup, 0, len(groups))
	for _, g := range groups {
		converted = append(converted, toCandidateGroup(g))
	}
	return encodeCandidateGroupList(converted)
}

func encodeCandidateGroupList(groups []flow.CandidateGroup) []interface{} {
	return flow.EncodeCandidateGroups(groups)
}

type ChangeAsk struct {
	pickedField bool
	gaveValue   bool
	answer      string
	field       string
}

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

func proceedToUpdateConfirm(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movement.MovementRow, ask ChangeAsk) error {
	if ask.pickedField && !ask.gaveValue {
		return parkChangeQuestion(ctx, svc, chat, userID, message, transactionID, oldIDs, beforeRows, ask)
	}

	if after, ok := amountOnlyCorrection(beforeRows, ask); ok {
		return seedAndStartUpdateConfirm(ctx, svc, chat, userID, message, oldIDs, beforeRows,
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
	if !result.Resolved || correctionIsNoOp(beforeRows, result.Movements) {
		if ask.gaveValue {
			resolveMetric(ctx, svc, userID, outcomeLoopDidNothing)
			svc.SendText(ctx, chat, msgStillCannotCorrect)
			return nil
		}
		return parkChangeQuestion(ctx, svc, chat, userID, message, transactionID, oldIDs, beforeRows, ask)
	}

	return seedAndStartUpdateConfirm(ctx, svc, chat, userID, message, oldIDs, beforeRows, result)
}

func correctionIsNoOp(before []movement.MovementRow, after []orchestrator.MovementDraft) bool {
	if len(before) == 0 || len(before) != len(after) {
		return false
	}
	for i, draft := range after {
		if !sameMovementForCorrection(before[i], draftToRow(draft)) {
			return false
		}
	}
	return true
}

func sameMovementForCorrection(before, after movement.MovementRow) bool {
	unchanged := func(b, a string) bool { return a == "" || a == b }
	return sameAmount(before.Amount, after.Amount) &&
		unchanged(before.Type, after.Type) &&
		unchanged(before.Currency, after.Currency) &&
		unchanged(before.Category, after.Category) &&
		unchanged(before.Subcategory, after.Subcategory) &&
		unchanged(before.Date, after.Date) &&
		unchanged(before.AccountID, after.AccountID) &&
		unchanged(before.AccountNameGuess, after.AccountNameGuess) &&
		unchanged(before.Description, after.Description)
}

func sameAmount(before, after string) bool {
	if after == "" {
		return true
	}
	b, berr := movement.ParseARAmount(before)
	a, aerr := movement.ParseARAmount(after)
	if berr != nil || aerr != nil {
		return before == after
	}
	return b.Abs().Equal(a.Abs())
}

func askWhatToChange(rows []movement.MovementRow) string {
	const ask = "¿Cuánto era? Escribime el monto — o tocá abajo si lo que está mal es otra cosa."
	if len(rows) == 0 {
		return ask
	}
	return "Encontré " + movement.MovementGapDescriptor(rows[0]) + ". " + ask
}

func parkChangeQuestion(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, change, transactionID string, oldIDs []string, rows []movement.MovementRow, ask ChangeAsk) error {
	if !svc.ActionsEnabled() {
		resolveMetric(ctx, svc, userID, outcomeParkFailed)
		svc.SendText(ctx, chat, flow.MsgSomethingBroke)
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
	question := pendingaction.OpenQuestion{
		Key:     questionKeyChange,
		Prompt:  askWhatToChange(rows),
		Options: changeFieldOptions(),
	}
	if ask.pickedField {
		question.Prompt, question.Options = "Dale. ¿Y cuál es el valor nuevo?", nil
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
	return drainNextAgentAction(ctx, svc, chat, userID)
}

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

func applyStructuredCorrection(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, payload agentPayload, groups []flow.CandidateGroup) error {
	before := make([][]movement.MovementRow, 0, len(groups))
	var oldIDs []string
	for _, g := range groups {
		before = append(before, g.Rows)
		oldIDs = append(oldIDs, g.OldIDs...)
	}

	after, err := applyChangesToSet(before, payload.Changes, guardContext{
		Scope:        payload.Scope,
		NamedAccount: accountNamedIn(svc, userID, payload.Change),
		Message:      payload.Change,
	})
	if err != nil {
		slog.WarnContext(ctx, "structured correction rejected", "user_id", userID, "err", err)
		resolveMetric(ctx, svc, userID, outcomeCorrectionRefused)
		if errors.Is(err, errRefundThatGrows) {
			svc.SendText(ctx, chat, "Me dijiste que te devolvieron plata, pero el cambio que entendí lo dejaría más caro. ¿Cuánto te devolvieron?")
			return nil
		}
		if errors.Is(err, errRefundExceedsAmount) {
			svc.SendText(ctx, chat, "Me decís que te devolvieron más de lo que salió ese movimiento 🤔 ¿Cuánto fue?")
			return nil
		}
		if errors.Is(err, errAmbiguousSetAll) {
			svc.SendText(ctx, chat, "¿A cuál de todos le pongo ese monto? Decime cuál y lo cambio.")
			return nil
		}
		svc.SendText(ctx, chat, msgStillCannotCorrect)
		return nil
	}

	taxonomy := userTaxonomy(svc, userID)
	accounts, _ := svc.FindUserAccounts(userID)
	beforeRows := make([]movement.MovementRow, 0, len(oldIDs))
	for _, g := range before {
		beforeRows = append(beforeRows, g...)
	}
	drafts := make([]orchestrator.MovementDraft, 0, len(beforeRows))
	for _, g := range after {
		for _, r := range g {
			if r.Subcategory == "" {
				if cat, sub, ok := resolveTaxonomy(r.Category, taxonomy); ok {
					r.Category, r.Subcategory = cat, sub
				}
			}
			if r.AccountID == "" && r.AccountNameGuess != "" {
				if id := matchNamedAccount(r.AccountNameGuess, accounts, r.Currency); id != 0 {
					r.AccountID = strconv.FormatUint(id, 10)
				}
			}
			drafts = append(drafts, rowToDraft(r))
		}
	}

	if correctionIsNoOp(beforeRows, drafts) {
		resolveMetric(ctx, svc, userID, outcomeNothingToChange)
		svc.SendText(ctx, chat, "Eso ya estaba así, no cambié nada.")
		return nil
	}

	return seedAndStartUpdateConfirm(ctx, svc, chat, userID, payload.Change, oldIDs, beforeRows,
		orchestrator.UpdateResult{Resolved: true, Movements: drafts})
}

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

func seedAndStartUpdateConfirm(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, userMessage string, oldIDs []string, beforeRows []movement.MovementRow, result orchestrator.UpdateResult) error {
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

	taxonomy := userTaxonomy(svc, userID)

	seed := conversation.Data{
		conversation.KeyMode:                modeUpdate,
		conversation.KeyOldMovementIDs:      conversation.EncodeStringSlice(oldIDs),
		conversation.KeyBeforeMovements:     movement.EncodeMovementRows(beforeRows),
		conversation.KeyMovements:           movement.EncodeMovementRows(afterRows),
		conversation.KeyPendingCategoryGaps: conversation.EncodeStringSlice(categoryGapsFor(afterRows, taxonomy)),
		conversation.KeyPendingAccountGaps:  conversation.EncodeStringSlice(accountGapsFor(afterRows)),
		conversation.KeyDeleteInstead:       strconv.FormatBool(correctionIsDeletion(afterRows, userMessage)),
	}

	flowName := flow.MovementUpdateConfirmFlowName
	if len(conversation.DecodeStringSlice(seed, conversation.KeyPendingCategoryGaps)) > 0 ||
		len(conversation.DecodeStringSlice(seed, conversation.KeyPendingAccountGaps)) > 0 {
		flowName = flow.MovementCreateFlowName
	}

	prompt, err := svc.EngineStartWithData(userID, flowName, seed)
	if err != nil {
		return err
	}
	svc.SendPrompt(ctx, chat, prompt)
	return nil
}

func finishMovementUpdatePickFlow(ctx context.Context, svc agentServices, chat messenger.Chat, data conversation.Data) {
	idx, err := strconv.Atoi(conversation.StringOrEmpty(data["chosen_index"]))
	if err != nil {
		_ = messenger.SendText(ctx, chat, flow.MsgSomethingBroke)
		return
	}

	candidates := flow.DecodeCandidateGroups(data)
	if idx < 0 || idx >= len(candidates) {
		_ = messenger.SendText(ctx, chat, flow.MsgSomethingBroke)
		return
	}
	chosen := candidates[idx]

	message := conversation.StringOrEmpty(data["message"])
	if err := proceedToUpdateConfirm(ctx, svc, chat, data.UserID(), message, chosen.TransactionID, chosen.OldIDs, chosen.Rows, ChangeAsk{}); err != nil {
		if svc.EnqueueUpdatePickIfRateLimited(ctx, chat, data.UserID(), message, chosen.TransactionID, chosen.OldIDs, chosen.Rows, err) {
			return
		}
		_ = messenger.SendText(ctx, chat, flow.MsgSomethingBroke)
	}
}

var palabrasDeMontoCero = []string{"gratis", "regal", "nada", "cero", "invit"}

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
