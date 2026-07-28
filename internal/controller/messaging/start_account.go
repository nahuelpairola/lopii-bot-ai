package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/go-telegram/bot"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/orchestrator"
)

// startAccountManage runs Call 2 account-match and branches: matched →
// manage menu; wants-new → the existing (prefill-seeded) create flow;
// unclear → the candidate picker. Candidates are always seeded — the pick
// step needs them, the menu path skips it via SkipIf.
func (c *controller) startAccountManage(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", accountManageFlowName, "user_id", userID)
	accs, err := c.accounts.FindByUserID(userID)
	if err != nil {
		c.sendText(ctx, b, chatID, msgCouldNotLoad)
		return fmt.Errorf("account manage: find accounts: %w", err)
	}
	if len(accs) == 0 {
		c.resolveMetric(ctx, userID, outcomeAccountCreateRouted)
		return c.startAccountCreate(ctx, b, chatID, userID, text)
	}

	opts := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		opts = append(opts, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}
	res, err := c.orchestrator.ResolveAccountManage(ctx, text, opts)
	if err != nil {
		if handled, oerr := c.handleGroqError(ctx, b, chatID, userID, text, err); handled {
			return oerr
		}
		c.sendText(ctx, b, chatID, msgSomethingBroke)
		return fmt.Errorf("account manage: resolve: %w", err)
	}
	if res.WantsNewAccount {
		c.resolveMetric(ctx, userID, outcomeAccountCreateRouted)
		return c.startAccountCreate(ctx, b, chatID, userID, text)
	}

	ids := make([]string, 0, len(accs))
	labels := make([]string, 0, len(accs))
	names := make([]string, 0, len(accs))
	curs := make([]string, 0, len(accs))
	for _, a := range accs {
		ids = append(ids, strconv.FormatUint(uint64(a.ID), 10))
		labels = append(labels, a.Name+" ("+a.Currency.String()+")")
		names = append(names, a.Name)
		curs = append(curs, a.Currency.String())
	}
	seed := conversation.Data{
		keyMessage:             text,
		keyCandidateIDs:        encodeStringSlice(ids),
		keyCandidateLabels:     encodeStringSlice(labels),
		keyCandidateNames:      encodeStringSlice(names),
		keyCandidateCurrencies: encodeStringSlice(curs),
	}
	if res.MatchedAccountID != nil {
		// never trust an LLM id blindly — it must exist in the user's list
		for _, a := range accs {
			if uint64(a.ID) == *res.MatchedAccountID {
				seed[keyAccountID] = strconv.FormatUint(uint64(a.ID), 10)
				seed[keyAccountName] = a.Name
				seed[keyAccountCurrency] = a.Currency.String()
				break
			}
		}
	}

	return c.startFlow(ctx, b, chatID, userID, accountManageFlowName, seed, "account manage: start account_manage flow")
}

func (c *controller) startAccountCreate(ctx context.Context, b *bot.Bot, chatID int64, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", accountCreateFlowName, "user_id", userID)
	seed := c.accountCreateSeed(ctx, text)
	return c.startFlow(ctx, b, chatID, userID, accountCreateFlowName, seed, "start account_create flow")
}

// accountCreateSeed reuses ClassifyOnboarding to prefill the flow when the
// triggering message already states the account name and/or opening balance
// (e.g. "Nueva cuenta: Cedears tengo 1041265"). It seeds only when exactly one
// account is extracted; 0, >1, or an extractor error fall back to a blank flow
// (StartWithData with an empty seed == Start). Currency is never seeded — it
// stays the flow's currency ChoiceStep. No step is auto-skipped: the user still
// confirms every value.
//
// Returns an empty (non-nil) Data when nothing should be prefilled.
func (c *controller) accountCreateSeed(ctx context.Context, text string) conversation.Data {
	seed := conversation.Data{}
	res, err := c.orchestrator.ClassifyOnboarding(ctx, text)
	if err != nil || len(res.Accounts) != 1 {
		return seed
	}
	d := res.Accounts[0]
	if d.Name != "" {
		seed[keyAccountName] = d.Name
	}
	if amt, err := parseARAmount(d.Balance); err == nil && !amt.IsNegative() {
		seed[keyAccountBalance] = d.Balance
	}
	return seed
}
