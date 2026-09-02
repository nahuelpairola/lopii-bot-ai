package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
)

func startAgentLoop(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, text string) error {
	_ = chat.Typing(ctx)

	tools := wiredAgentTools()
	prompt, taxonomy, err := buildAgentSystemPrompt(svc, userID, tools)
	if err != nil {
		svc.SendText(ctx, chat, flow.MsgCouldNotLoad)
		return err
	}

	turns, _ := svc.ChatHistoryRecent(userID)
	history := make([]orchestrator.QueryTurn, len(turns))
	for i, t := range turns {
		history[i] = orchestrator.QueryTurn{Question: t.Question, Answer: t.Answer}
	}

	executor := newAgentExecutor(ctx, svc, userID, text, taxonomy)
	answer, err := svc.Run(ctx, prompt, text, history, tools, executor.execute)

	if svc.IsReplaying(ctx) {
		setQueuedIntent(ctx, svc, userID, intentForExecutor(executor, err))
	} else {
		logIntent(ctx, svc, userID, text, intentForExecutor(executor, err), err)
	}

	if err != nil {
		if executor.wrote {
			resolveMetric(ctx, svc, userID, flow.OutcomeCreateInserted, collectMovementIDs(executor.inserted)...)
			svc.SendText(ctx, chat, "Registré lo que me pediste, pero me quedé sin margen para el resto. Mandame de nuevo lo que falte.")
			slog.WarnContext(ctx, "agent loop failed after a write: not queued", "user_id", userID, "err", err)
			return nil
		}
		if handled, oerr := svc.HandleGroqError(ctx, chat, userID, text, err); handled {
			return oerr
		}
		resolveMetric(ctx, svc, userID, outcomeLoopErrored)
		slog.ErrorContext(ctx, "agent loop failed", "user_id", userID, "err", err)
		svc.SendText(ctx, chat, flow.MsgSomethingBroke)
		return fmt.Errorf("agent loop: %w", err)
	}

	if executor.answerQuery {
		return svc.FinishAnswerQuery(ctx, chat, userID, text)
	}
	if executor.settingsArea != "" {
		return svc.FinishManageSettings(ctx, chat, userID, text, executor.settingsArea)
	}

	if executor.reply != "" {
		if len(executor.replyButtons) > 0 {
			svc.SendPrompt(ctx, chat, conversation.Prompt{Text: executor.reply, Buttons: executor.replyButtons})
		} else {
			svc.SendText(ctx, chat, executor.reply)
		}
	} else if narration := strings.TrimSpace(answer); narration != "" {
		svc.SendText(ctx, chat, narration)
	}

	if len(executor.parked) > 0 {
		if err := parkAgentActions(ctx, svc, userID, executor.parked); err != nil {
			resolveMetric(ctx, svc, userID, outcomeParkFailed)
			slog.ErrorContext(ctx, "park agent actions failed", "user_id", userID, "err", err)
			svc.SendText(ctx, chat, flow.MsgSomethingBroke)
			return err
		}
	}
	if reply := firstNonEmpty(executor.reply, strings.TrimSpace(answer)); reply != "" {
		if err := svc.ChatHistoryAppend(userID, text, reply); err != nil {
			slog.WarnContext(ctx, "chat history append failed", "user_id", userID, "err", err)
		}
	}

	resolveAgentLoopMetric(ctx, svc, userID, executor)

	return drainNextAgentAction(ctx, svc, chat, userID)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func resolveAgentLoopMetric(ctx context.Context, svc agentServices, userID uint64, ex *agentExecutor) {
	if len(ex.parked) > 0 {
		return
	}
	switch {
	case len(ex.inserted) > 0:
		resolveMetric(ctx, svc, userID, flow.OutcomeCreateInserted, collectMovementIDs(ex.inserted)...)
	case ex.reply == msgHelp:
		resolveMetric(ctx, svc, userID, outcomeHelpShown)
	case ex.reply == MsgAskRewrite:
		resolveMetric(ctx, svc, userID, outcomeUnclear)
	case ex.noCandidates:
		resolveMetric(ctx, svc, userID, outcomeNoCandidates)
	default:
		resolveMetric(ctx, svc, userID, outcomeLoopDidNothing)
	}
}

func buildAgentSystemPrompt(svc agentServices, userID uint64, tools []orchestrator.AgentTool) (string, []orchestrator.TaxonomyEntry, error) {
	subs, err := svc.SubcategoriesFindAllForUser(userID)
	if err != nil {
		return "", nil, fmt.Errorf("agent loop: find subcategories: %w", err)
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, s := range subs {
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: s.Category, Subcategory: s.Subcategory, Description: s.Description})
	}

	accs, err := svc.FindUserAccounts(userID)
	if err != nil {
		return "", nil, fmt.Errorf("agent loop: find accounts: %w", err)
	}
	accountOptions := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		accountOptions = append(accountOptions, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}

	today := movement.TodayCivil()
	return orchestrator.BuildAgentPrompt(movement.WeekdayEs(today)+" "+today.Format("2006-01-02"), accountOptions, taxonomy, "", tools,
		BuildRecentEntities(svc, userID)), taxonomy, nil
}
