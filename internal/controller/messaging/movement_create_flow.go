package messaging

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

const (
	movementCreateFlowName = "movement_create"

	stepResolveCategory    = "resolve_category"
	stepResolveSubcategory = "resolve_subcategory"
	stepResolveAccount     = "resolve_account"
)

// NewMovementCreateFlow builds the single registered flow used for
// CREATE's gap-fill (and, per movement_update_flow.go, for reusing the
// same graph to fill gaps in an UPDATE's corrected set). It is only
// ever started via StartWithData when Call 2 CREATE left at least one
// gap — a fully-resolved CREATE never touches the conversation engine
// at all (see free_text.go).
func NewMovementCreateFlow(subcategories subcategoryRepository, accounts accountRepository) *conversation.Flow {
	steps := map[string]conversation.Step{
		stepResolveCategory: conversation.ChoiceStep{
			PromptText: msgAskCategory,
			SkipIf: func(data conversation.Data) (string, bool) {
				if len(decodeStringSlice(data, "pending_category_gaps")) == 0 {
					return stepResolveAccount, true
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				cats, _ := subcategories.DistinctCategoriesForUser(data.UserID())
				opts := make([]conversation.ChoiceOption, 0, len(cats))
				for _, cat := range cats {
					opts = append(opts, conversation.ChoiceOption{
						Label:    subcategory.IconFor(cat) + " " + cat,
						Value:    cat,
						NextStep: stepResolveSubcategory,
					})
				}
				return opts
			},
			DeclaredNextSteps: []string{stepResolveSubcategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				gaps := decodeStringSlice(data, "pending_category_gaps")
				if len(gaps) == 0 {
					return data
				}
				next := copyData(data)
				next["gap_active_row"] = gaps[0]
				rows := decodeMovementRows(data)
				idx, _ := strconv.Atoi(gaps[0])
				rows[idx].Category = value
				next["movements"] = encodeMovementRows(rows)
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
		stepResolveSubcategory: conversation.ChoiceStep{
			PromptText: msgAskSubcategory,
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				rowIdx, _ := strconv.Atoi(stringOrEmpty(data["gap_active_row"]))
				rows := decodeMovementRows(data)
				category := rows[rowIdx].Category

				subs, _ := subcategories.FindAllForUser(data.UserID())
				var opts []conversation.ChoiceOption
				for _, s := range subs {
					if s.Category != category {
						continue
					}
					opts = append(opts, conversation.ChoiceOption{
						Label:    s.Subcategory,
						Value:    s.Subcategory,
						NextStep: stepResolveCategory,
					})
				}
				return opts
			},
			DeclaredNextSteps: []string{stepResolveCategory},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				gaps := decodeStringSlice(data, "pending_category_gaps")
				rowIdx, _ := strconv.Atoi(stringOrEmpty(data["gap_active_row"]))

				rows := decodeMovementRows(data)
				rows[rowIdx].Subcategory = value
				next["movements"] = encodeMovementRows(rows)
				if len(gaps) > 0 {
					next["pending_category_gaps"] = encodeStringSlice(gaps[1:])
				}
				next["gap_active_row"] = ""
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
		stepResolveAccount: conversation.ChoiceStep{
			PromptText: msgAskAccount,
			SkipIf: func(data conversation.Data) (string, bool) {
				if len(decodeStringSlice(data, "pending_account_gaps")) == 0 {
					return "", true // nothing left — the flow is complete
				}
				return "", false
			},
			OptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				gaps := decodeStringSlice(data, "pending_account_gaps")
				if len(gaps) == 0 {
					return nil
				}
				rowIdx, _ := strconv.Atoi(gaps[0])
				rows := decodeMovementRows(data)

				accs, _ := accounts.FindByUserID(data.UserID())
				var opts []conversation.ChoiceOption
				for _, a := range accs {
					if a.Currency.String() != rows[rowIdx].Currency {
						continue
					}
					opts = append(opts, conversation.ChoiceOption{
						Label:    a.Name,
						Value:    "existing:" + strconv.FormatUint(uint64(a.ID), 10),
						NextStep: stepResolveAccount,
					})
				}
				opts = append(opts, conversation.ChoiceOption{
					Label:    "➕ Crear cuenta \"" + rows[rowIdx].AccountNameGuess + "\"",
					Value:    "create",
					NextStep: stepResolveAccount,
				})
				return opts
			},
			DeclaredNextSteps: []string{stepResolveAccount},
			OnChoice: func(value string, data conversation.Data) conversation.Data {
				next := copyData(data)
				gaps := decodeStringSlice(data, "pending_account_gaps")
				if len(gaps) == 0 {
					return next
				}
				rowIdx, _ := strconv.Atoi(gaps[0])

				rows := decodeMovementRows(data)
				if value == "create" {
					rows[rowIdx].AccountID = accountPendingCreate
				} else {
					rows[rowIdx].AccountID = strings.TrimPrefix(value, "existing:")
				}
				next["movements"] = encodeMovementRows(rows)
				next["pending_account_gaps"] = encodeStringSlice(gaps[1:])
				return next
			},
			InvalidChoiceMessage: msgGenericFlowError,
		},
	}

	flow, err := conversation.NewFlow(movementCreateFlowName, stepResolveCategory, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// finishMovementCreateFlow is the Telegram-facing wrapper around
// resolveAndInsertMovements — same split for testability as
// finishInitialBalanceFlow/insertInitialBalanceMovements.
func (c *controller) finishMovementCreateFlow(ctx context.Context, b *bot.Bot, chatID int64, data conversation.Data) {
	inserted, err := c.resolveAndInsertMovements(data)
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgGenericFlowError})
		return
	}
	c.lastTransactions.Set(data.UserID(), inserted)
	b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: msgConfirmMovements(inserted)})
}

// resolveAndInsertMovements does the real work: creates any
// account_pending_create accounts (deduped by name+currency), parses
// every movementRow into a movement.Movement, assigns a shared
// transaction_id only when there's more than one row, computes the FCI
// redemption gain (app code, never the LLM — see the CREATE spec's
// deterministic full-vs-partial-redemption rule), and inserts
// everything via InsertBatch (mode=create) or ReplaceMovements
// (mode=update, replacing data["old_movement_ids"] — see
// movement_update_flow.go).
func (c *controller) resolveAndInsertMovements(data conversation.Data) ([]movement.Movement, error) {
	userID := data.UserID()
	rows := decodeMovementRows(data)

	createdAccounts := make(map[string]uint64) // "name|currency" -> new account id
	for i, row := range rows {
		if row.AccountID != accountPendingCreate {
			continue
		}
		key := row.AccountNameGuess + "|" + row.Currency
		if id, ok := createdAccounts[key]; ok {
			rows[i].AccountID = strconv.FormatUint(id, 10)
			continue
		}
		newAccount := &account.Account{
			UserID:   userID,
			Name:     row.AccountNameGuess,
			Currency: currency.Currency(row.Currency),
		}
		if err := c.accounts.Insert(newAccount); err != nil {
			return nil, err
		}
		id := uint64(newAccount.ID)
		createdAccounts[key] = id
		rows[i].AccountID = strconv.FormatUint(id, 10)
	}

	movements := make([]movement.Movement, 0, len(rows)+1)
	var transactionID *uuid.UUID
	if len(rows) > 1 {
		id := uuid.New()
		transactionID = &id
	}

	for _, row := range rows {
		sub, err := c.subcategories.FindByCategoryAndSubcategory(row.Category, row.Subcategory)
		if err != nil {
			return nil, err
		}
		amount, err := decimal.NewFromString(row.Amount)
		if err != nil {
			return nil, err
		}
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			return nil, err
		}

		var accountID *uint64
		if row.AccountID != "" {
			id, err := strconv.ParseUint(row.AccountID, 10, 64)
			if err != nil {
				return nil, err
			}
			accountID = &id
		}

		m := movement.Movement{
			TransactionID: transactionID,
			UserID:        userID,
			AccountID:     accountID,
			SubcategoryID: uint64(sub.ID),
			Date:          date,
			Type:          movement.TypeFromString(row.Type),
			Amount:        amount,
			Currency:      currency.Currency(row.Currency),
			PaymentMethod: optionalString(row.PaymentMethod),
			Merchant:      optionalString(row.Merchant),
			Description:   optionalString(row.Description),
		}
		movements = append(movements, m)
	}

	gain, ok, err := fciRedemptionGain(c, movements)
	if err != nil {
		return nil, err
	}
	if ok {
		movements = append(movements, gain)
	}

	mode := stringOrEmpty(data["mode"])
	if mode == "update" {
		oldIDs, err := parseUintSlice(decodeStringSlice(data, "old_movement_ids"))
		if err != nil {
			return nil, err
		}
		if err := c.movements.ReplaceMovements(oldIDs, movements); err != nil {
			return nil, err
		}
		return movements, nil
	}

	if err := c.movements.InsertBatch(movements); err != nil {
		return nil, err
	}
	return movements, nil
}

// fciRedemptionGain detects an FCI-redemption outflow leg among the
// movements about to be inserted and, per the spec's deterministic
// rule, computes a gain movement only when the redeemed amount is at
// least the account's balance before this transaction. Never guesses a
// number the app can't actually justify.
//
// A negative-amount transfer under Inversiones|FCI is NOT enough to
// identify a redemption: an FCI *subscription* (money leaving the
// wallet to invest) has the exact same shape (same type, same negative
// sign, same subcategory — there's only one Inversiones|FCI
// subcategory, no separate buy/sell). The disambiguator is
// Account.IsDefault: an FCI account is almost never the user's default
// (everyday) wallet, so a negative leg on the default account is a
// subscription, never a redemption candidate.
//
// Repository errors on lookups that matter once a movement otherwise
// looks like a genuine redemption candidate (subcategory resolution,
// balance lookup, gain-subcategory resolution) are propagated instead
// of silently treated as "not a candidate" — a config problem (e.g. a
// missing reserved subcategory) or a transient DB failure should never
// silently drop a real gain.
func fciRedemptionGain(c *controller, movements []movement.Movement) (movement.Movement, bool, error) {
	for _, m := range movements {
		if m.Type != movement.Transfer || m.AccountID == nil {
			continue
		}
		if !m.Amount.IsNegative() {
			continue
		}

		fciSub, err := c.subcategories.FindByCategoryAndSubcategory("Inversiones", "FCI")
		if err != nil {
			return movement.Movement{}, false, err
		}
		if m.SubcategoryID != uint64(fciSub.ID) {
			continue
		}

		acc, err := c.accounts.GetAccount(*m.AccountID)
		if err != nil || acc.IsDefault {
			continue // default (everyday) account: a subscription, not a redemption
		}

		balanceBefore, err := c.movements.SumAmountForAccount(*m.AccountID)
		if err != nil {
			return movement.Movement{}, false, err
		}
		redeemed := m.Amount.Neg()
		if redeemed.LessThan(balanceBefore) {
			continue
		}
		gain := redeemed.Sub(balanceBefore)
		if !gain.IsPositive() {
			continue
		}

		gainSub, err := c.subcategories.FindByCategoryAndSubcategory("Sistema", "Rendimiento inversión")
		if err != nil {
			return movement.Movement{}, false, err
		}
		return movement.Movement{
			TransactionID: m.TransactionID,
			UserID:        m.UserID,
			AccountID:     nil,
			SubcategoryID: uint64(gainSub.ID),
			Date:          m.Date,
			Type:          movement.Income,
			Amount:        gain,
			Currency:      m.Currency,
		}, true, nil
	}
	return movement.Movement{}, false, nil
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
