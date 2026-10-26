package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingjob"
)

func (l *Loop) StartLoop(ctx context.Context, chat messenger.Chat, userID uint64, text string) error {
	_ = chat.Typing(ctx)

	tools := wiredAgentTools()
	prompt, taxonomy, err := l.buildAgentSystemPrompt(userID, tools)
	if err != nil {
		flow.SendText(ctx, chat, flow.MsgCouldNotLoad)
		return err
	}

	turns, _ := l.chatHistory.Recent(userID)
	history := make([]orchestrator.QueryTurn, len(turns))
	for i, t := range turns {
		history[i] = orchestrator.QueryTurn{Question: t.Question, Answer: t.Answer}
	}

	executor := l.newAgentExecutor(ctx, userID, text, taxonomy)
	answer, err := l.model.Run(ctx, prompt, text, history, tools, executor.execute)

	if executor.claimErr != nil {
		return executor.claimErr
	}
	var rl *orchestrator.RateLimitedError
	if !errors.As(err, &rl) {
		if cerr := pendingjob.ClaimReplay(ctx); cerr != nil {
			return cerr
		}
	}

	if pendingjob.IsReplaying(ctx) {
		l.setQueuedIntent(ctx, userID, intentForExecutor(executor, err))
	} else {
		l.logIntent(ctx, userID, text, intentForExecutor(executor, err), err)
	}

	if err != nil {
		if executor.wrote {
			l.resolveMetric(ctx, userID, flow.OutcomeCreateInserted, collectMovementIDs(executor.inserted)...)
			flow.SendText(ctx, chat, "Registré lo que me pediste, pero me quedé sin margen para el resto. Mandame de nuevo lo que falte.")
			slog.WarnContext(ctx, "agent loop failed after a write: not queued", "user_id", userID, "err", err)
			return nil
		}
		if handled, oerr := l.handleGroqError(ctx, chat, userID, text, err); handled {
			return oerr
		}
		l.resolveMetric(ctx, userID, outcomeLoopErrored)
		slog.ErrorContext(ctx, "agent loop failed", "user_id", userID, "err", err)
		flow.SendText(ctx, chat, flow.MsgSomethingBroke)
		return fmt.Errorf("agent loop: %w", err)
	}

	if executor.answerQuery {
		return l.answerQuery(ctx, chat, userID, text)
	}
	if executor.settingsArea != "" {
		return l.manageSettings(ctx, chat, userID, text, executor.settingsArea)
	}

	if executor.reply != "" {
		if len(executor.replyButtons) > 0 {
			flow.SendPrompt(ctx, chat, conversation.Prompt{Text: executor.reply, Buttons: executor.replyButtons})
		} else {
			flow.SendText(ctx, chat, executor.reply)
		}
	} else if narration := strings.TrimSpace(answer); narration != "" {
		flow.SendText(ctx, chat, narration)
	}

	if len(executor.parked) > 0 {
		if err := l.parkAgentActions(ctx, userID, executor.parked); err != nil {
			l.resolveMetric(ctx, userID, outcomeParkFailed)
			slog.ErrorContext(ctx, "park agent actions failed", "user_id", userID, "err", err)
			flow.SendText(ctx, chat, flow.MsgSomethingBroke)
			return err
		}
	}
	if reply := firstNonEmpty(executor.reply, strings.TrimSpace(answer)); reply != "" {
		if err := l.chatHistory.Append(userID, text, reply); err != nil {
			slog.WarnContext(ctx, "chat history append failed", "user_id", userID, "err", err)
		}
	}

	l.resolveAgentLoopMetric(ctx, userID, executor)

	return l.DrainNextAction(ctx, chat, userID)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func (l *Loop) resolveAgentLoopMetric(ctx context.Context, userID uint64, ex *agentExecutor) {
	if len(ex.parked) > 0 {
		return
	}
	switch {
	case len(ex.inserted) > 0:
		l.resolveMetric(ctx, userID, flow.OutcomeCreateInserted, collectMovementIDs(ex.inserted)...)
	case ex.reply == msgHelp:
		l.resolveMetric(ctx, userID, outcomeHelpShown)
	case ex.reply == MsgAskRewrite:
		l.resolveMetric(ctx, userID, outcomeUnclear)
	case ex.noCandidates:
		l.resolveMetric(ctx, userID, outcomeNoCandidates)
	default:
		l.resolveMetric(ctx, userID, outcomeLoopDidNothing)
	}
}

func (l *Loop) buildAgentSystemPrompt(userID uint64, tools []orchestrator.AgentTool) (string, []orchestrator.TaxonomyEntry, error) {
	subs, err := l.subcategories.FindAllForUser(userID)
	if err != nil {
		return "", nil, fmt.Errorf("agent loop: find subcategories: %w", err)
	}
	taxonomy := make([]orchestrator.TaxonomyEntry, 0, len(subs))
	for _, s := range subs {
		taxonomy = append(taxonomy, orchestrator.TaxonomyEntry{Category: s.Category, Subcategory: s.Subcategory, Description: s.Description})
	}

	accs, err := l.accounts.FindByUserID(userID)
	if err != nil {
		return "", nil, fmt.Errorf("agent loop: find accounts: %w", err)
	}
	accountOptions := make([]orchestrator.AccountOption, 0, len(accs))
	for _, a := range accs {
		accountOptions = append(accountOptions, orchestrator.AccountOption{ID: uint64(a.ID), Name: a.Name, Currency: a.Currency.String()})
	}

	today := movement.TodayCivil()
	return orchestrator.BuildAgentPrompt(movement.WeekdayEs(today)+" "+today.Format("2006-01-02"), accountOptions, taxonomy, "", tools,
		l.BuildRecentEntities(userID)), taxonomy, nil
}
