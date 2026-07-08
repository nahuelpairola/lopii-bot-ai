package messaging

import (
	"context"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

const (
	movementUpdatePickFlowName    = "movement_update_pick"
	movementUpdateConfirmFlowName = "movement_update_confirm"

	stepPickUpdateCandidate = "pick_update_candidate"
	stepConfirmUpdate       = "confirm_update"
)

// NewMovementUpdatePickFlow is only ever started when reference
// resolution found 2+ ambiguous candidates (see free_text.go, Task 18)
// — a single resolved candidate skips straight to
// NewMovementUpdateConfirmFlow via proceedToUpdateConfirm.
func NewMovementUpdatePickFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepPickUpdateCandidate: conversation.ChoiceStep{
			PromptText: msgPickUpdateCandidate,
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				labels := decodeStringSlice(data, "candidate_labels")
				opts := make([]conversation.ChoiceOption, 0, len(labels))
				for i, label := range labels {
					opts = append(opts, conversation.ChoiceOption{
						Label:  label,
						Value:  strconv.Itoa(i),
						Finish: true,
					})
				}
				return opts
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				next["chosen_index"] = value
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}

	flow, err := conversation.NewFlow(movementUpdatePickFlowName, stepPickUpdateCandidate, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// NewMovementUpdateConfirmFlow is a single confirm/cancel gate — always
// reached before an UPDATE touches the DB, whether the candidate was a
// single unambiguous resolveCandidates match or picked from a list.
func NewMovementUpdateConfirmFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepConfirmUpdate: conversation.ChoiceStep{
			PromptText: msgConfirmUpdateDiff,
			Options: []conversation.ChoiceOption{
				{Label: "✅ Confirmar", Value: "confirm", Finish: true},
				{Label: "❌ Cancelar", Value: "cancel", Finish: true},
			},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				next["confirmed"] = strconv.FormatBool(value == "confirm")
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}

	flow, err := conversation.NewFlow(movementUpdateConfirmFlowName, stepConfirmUpdate, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

func movementToRow(m movement.Movement) movementRow {
	row := movementRow{
		Type:     string(m.Type),
		Amount:   displayAmount(m.Amount),
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
	if m.PaymentMethod != nil {
		row.PaymentMethod = *m.PaymentMethod
	}
	if m.Merchant != nil {
		row.Merchant = *m.Merchant
	}
	if m.Description != nil {
		row.Description = *m.Description
	}
	return row
}

func rowToDraft(r movementRow) orchestrator.MovementDraft {
	draft := orchestrator.MovementDraft{
		Type:             r.Type,
		Amount:           r.Amount,
		Currency:         r.Currency,
		AccountNameGuess: r.AccountNameGuess,
		Category:         r.Category,
		Subcategory:      r.Subcategory,
		PaymentMethod:    r.PaymentMethod,
		Merchant:         r.Merchant,
		Description:      r.Description,
		Date:             r.Date,
		Group:            r.Group,
	}
	if r.AccountID != "" && r.AccountID != accountPendingCreate {
		if id, err := strconv.ParseUint(r.AccountID, 10, 64); err == nil {
			draft.AccountID = &id
		}
	}
	return draft
}

func draftToRow(d orchestrator.MovementDraft) movementRow {
	row := movementRow{
		Type:             d.Type,
		Amount:           d.Amount,
		Currency:         d.Currency,
		AccountNameGuess: d.AccountNameGuess,
		Category:         d.Category,
		Subcategory:      d.Subcategory,
		PaymentMethod:    d.PaymentMethod,
		Merchant:         d.Merchant,
		Description:      d.Description,
		Date:             d.Date,
		Group:            d.Group,
	}
	if d.AccountID != nil {
		row.AccountID = strconv.FormatUint(*d.AccountID, 10)
	}
	return row
}

// candidateGroup is the row-based shape a picker candidate travels in
// through conversation.Data — distinct from transactionGroup (Task 15),
// which holds real movement.Movement rows straight from the DB.
type candidateGroup struct {
	TransactionID string
	OldIDs        []string
	Rows          []movementRow
}

// encodeCandidateGroups converts freshly-searched transactionGroups
// into their row-based Data shape, so the ambiguous-candidate picker
// (movement_update_pick) can carry full "before" state for whichever
// one the user ends up choosing, without a second DB round-trip.
func encodeCandidateGroups(groups []transactionGroup) []interface{} {
	encoded := make([]interface{}, 0, len(groups))
	for _, g := range groups {
		rows := make([]movementRow, 0, len(g.Movements))
		ids := make([]string, 0, len(g.Movements))
		for _, m := range g.Movements {
			rows = append(rows, movementToRow(m))
			ids = append(ids, strconv.FormatUint(uint64(m.ID), 10))
		}
		encoded = append(encoded, map[string]interface{}{
			"transaction_id": g.TransactionID,
			"old_ids":        encodeStringSlice(ids),
			"rows":           encodeMovementRows(rows),
		})
	}
	return encoded
}

func decodeCandidateGroups(data conversation.Data) []candidateGroup {
	raw, _ := data["candidate_groups"].([]interface{})
	groups := make([]candidateGroup, 0, len(raw))
	for _, r := range raw {
		m, _ := r.(map[string]interface{})
		groups = append(groups, candidateGroup{
			TransactionID: stringOrEmpty(m["transaction_id"]),
			OldIDs:        decodeStringSlice(conversation.Data{"ids": m["old_ids"]}, "ids"),
			Rows:          decodeMovementRows(conversation.Data{"movements": m["rows"]}),
		})
	}
	return groups
}

// proceedToUpdateConfirm runs Call 2 UPDATE against a candidate found
// via resolveCandidates — both the single-match path and the
// post-picker path funnel through here.
func (c *controller) proceedToUpdateConfirm(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, message, transactionID string, oldIDs []string, beforeRows []movementRow) error {
	drafts := make([]orchestrator.MovementDraft, 0, len(beforeRows))
	for _, row := range beforeRows {
		drafts = append(drafts, rowToDraft(row))
	}

	result, err := c.orchestrator.ResolveUpdate(ctx, message, orchestrator.MovementCandidate{
		TransactionID: transactionID,
		Movements:     drafts,
	})
	if err != nil {
		return err
	}
	if !result.Resolved {
		c.resolveMetric(userID, outcomeNoCandidates)
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgNoCandidatesFound})
		}
		return nil
	}

	return c.seedAndStartUpdateConfirm(ctx, b, chatID, userID, oldIDs, beforeRows, result)
}

// seedAndStartUpdateConfirm builds the confirm flow's seed from an
// already-resolved UpdateResult (never calls the orchestrator itself)
// and starts it. Called by proceedToUpdateConfirm once Call 2 UPDATE
// resolves.
func (c *controller) seedAndStartUpdateConfirm(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, oldIDs []string, beforeRows []movementRow, result orchestrator.UpdateResult) error {
	afterRows := make([]movementRow, 0, len(result.Movements))
	for _, d := range result.Movements {
		row := draftToRow(d)
		if sub, err := c.subcategories.FindByCategoryAndSubcategory(userID, row.Category, row.Subcategory); err == nil {
			row.Icon = sub.Icon
		}
		afterRows = append(afterRows, row)
	}

	seed := conversation.Data{
		"mode":                  "update",
		"old_movement_ids":      encodeStringSlice(oldIDs),
		"before_movements":      encodeMovementRows(beforeRows),
		"movements":             encodeMovementRows(afterRows),
		"pending_category_gaps": encodeStringSlice(nil),
		"pending_account_gaps":  encodeStringSlice(nil),
	}

	prompt, err := c.engine.StartWithData(userID, movementUpdateConfirmFlowName, seed)
	if err != nil {
		return err
	}
	if b != nil {
		c.sendPrompt(ctx, b, chatID, prompt)
	}
	return nil
}

// finishMovementUpdatePickFlow runs once the user has picked a
// candidate from an ambiguous list — it resolves the index back to the
// full candidate (both were seeded together) and hands off to
// proceedToUpdateConfirm.
func (c *controller) finishMovementUpdatePickFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	idx, err := strconv.Atoi(stringOrEmpty(data["chosen_index"]))
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
		return
	}

	candidates := decodeCandidateGroups(data)
	if idx < 0 || idx >= len(candidates) {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
		return
	}
	chosen := candidates[idx]

	message := stringOrEmpty(data["message"])
	if err := c.proceedToUpdateConfirm(ctx, b, chatID, data.UserID(), message, chosen.TransactionID, chosen.OldIDs, chosen.Rows); err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
	}
}

// finishMovementUpdateConfirmFlow applies (or discards) the correction
// depending on which button the user pressed — never both, never
// neither: the confirm ChoiceStep always finishes with "confirmed" set
// to one of "true"/"false".
func (c *controller) finishMovementUpdateConfirmFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	if stringOrEmpty(data["confirmed"]) != "true" {
		c.resolveMetric(data.UserID(), outcomeUpdateCancelled)
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgUpdateCancelled})
		}
		return
	}

	inserted, err := c.resolveAndInsertMovements(data)
	if err != nil {
		if b != nil {
			b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
		}
		return
	}
	c.resolveMetric(data.UserID(), outcomeUpdateConfirmed, collectMovementIDs(inserted)...)
	if b != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgUpdateApplied})
	}
}
