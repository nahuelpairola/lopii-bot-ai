package settings

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

func (w *Wizards) StartAccountManage(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.AccountManageFlowName, "user_id", userID)
	accs, err := w.accounts.FindByUserID(userID)
	if err != nil {
		flow.SendText(ctx, chat, flow.MsgCouldNotLoad)
		return fmt.Errorf("account manage: find accounts: %w", err)
	}
	if len(accs) == 0 {
		w.resolveMetric(ctx, userID, flow.OutcomeAccountCreateRouted)
		return w.StartAccountCreate(ctx, chat, userID, text)
	}

	opts := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		opts = append(opts, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}
	res, err := w.model.ResolveAccountManage(ctx, text, opts)
	if err != nil {
		if handled, oerr := w.handleGroqError(ctx, chat, userID, text, err); handled {
			return oerr
		}
		flow.SendText(ctx, chat, flow.MsgSomethingBroke)
		return fmt.Errorf("account manage: resolve: %w", err)
	}
	if res.WantsNewAccount {
		w.resolveMetric(ctx, userID, flow.OutcomeAccountCreateRouted)
		return w.StartAccountCreate(ctx, chat, userID, text)
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
		conversation.KeyMessage:             text,
		conversation.KeyCandidateIDs:        conversation.EncodeStringSlice(ids),
		conversation.KeyCandidateLabels:     conversation.EncodeStringSlice(labels),
		conversation.KeyCandidateNames:      conversation.EncodeStringSlice(names),
		conversation.KeyCandidateCurrencies: conversation.EncodeStringSlice(curs),
	}
	matched := res.MatchedAccountID
	if id, ok := accountNamedInText(text, accs); ok {
		matched = &id
	}
	if matched != nil {
		for _, a := range accs {
			if uint64(a.ID) == *matched {
				seed[conversation.KeyAccountID] = strconv.FormatUint(uint64(a.ID), 10)
				seed[conversation.KeyAccountName] = a.Name
				seed[conversation.KeyAccountCurrency] = a.Currency.String()
				break
			}
		}
	}
	_, resolved := seed[conversation.KeyAccountID]
	if adjust, total := adjustIntent(text, accs, resolved); adjust {
		seed[conversation.KeyOperationHint] = flow.OpAdjust
		if total != "" && flow.ValidateBalanceAmount(total, seed) == "" {
			seed[conversation.KeyNewTotal] = total
		}
	}

	return flow.StartFlow(ctx, w.engine, chat, userID, flow.AccountManageFlowName, seed, "account manage: start account_manage flow")
}

func (w *Wizards) StartAccountCreate(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.AccountCreateFlowName, "user_id", userID)
	seed := w.accountCreateSeed(ctx, text)
	return flow.StartFlow(ctx, w.engine, chat, userID, flow.AccountCreateFlowName, seed, "start account_create flow")
}

func (w *Wizards) accountCreateSeed(ctx context.Context, text string) conversation.Data {
	seed := conversation.Data{}
	res, err := w.model.ClassifyOnboarding(ctx, text)
	if err != nil || len(res.Accounts) != 1 {
		return seed
	}
	d := res.Accounts[0]
	if d.Name != "" {
		seed[conversation.KeyAccountName] = d.Name
	}
	if amt, err := movement.ParseARAmount(d.Balance); err == nil && !amt.IsNegative() {
		seed[conversation.KeyAccountBalance] = d.Balance
	}
	return seed
}
