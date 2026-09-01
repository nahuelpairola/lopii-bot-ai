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

func StartAccountManage(ctx context.Context, s Services, chat messenger.Chat, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.AccountManageFlowName, "user_id", userID)
	accs, err := s.FindUserAccounts(userID)
	if err != nil {
		s.SendText(ctx, chat, flow.MsgCouldNotLoad)
		return fmt.Errorf("account manage: find accounts: %w", err)
	}
	if len(accs) == 0 {
		s.ResolveMetric(ctx, userID, flow.OutcomeAccountCreateRouted)
		return StartAccountCreate(ctx, s, chat, userID, text)
	}

	opts := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		opts = append(opts, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}
	res, err := s.ResolveAccountManage(ctx, text, opts)
	if err != nil {
		if handled, oerr := s.HandleGroqError(ctx, chat, userID, text, err); handled {
			return oerr
		}
		s.SendText(ctx, chat, flow.MsgSomethingBroke)
		return fmt.Errorf("account manage: resolve: %w", err)
	}
	if res.WantsNewAccount {
		s.ResolveMetric(ctx, userID, flow.OutcomeAccountCreateRouted)
		return StartAccountCreate(ctx, s, chat, userID, text)
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
	if res.MatchedAccountID != nil {
		for _, a := range accs {
			if uint64(a.ID) == *res.MatchedAccountID {
				seed[conversation.KeyAccountID] = strconv.FormatUint(uint64(a.ID), 10)
				seed[conversation.KeyAccountName] = a.Name
				seed[conversation.KeyAccountCurrency] = a.Currency.String()
				break
			}
		}
	}

	return s.StartFlow(ctx, chat, userID, flow.AccountManageFlowName, seed, "account manage: start account_manage flow")
}

func StartAccountCreate(ctx context.Context, s Services, chat messenger.Chat, userID uint64, text string) error {
	slog.InfoContext(ctx, "flow started", "flow", flow.AccountCreateFlowName, "user_id", userID)
	seed := accountCreateSeed(ctx, s, text)
	return s.StartFlow(ctx, chat, userID, flow.AccountCreateFlowName, seed, "start account_create flow")
}

func accountCreateSeed(ctx context.Context, s Services, text string) conversation.Data {
	seed := conversation.Data{}
	res, err := s.ClassifyOnboarding(ctx, text)
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
