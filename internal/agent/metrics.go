package agent

import (
	"context"
	"errors"
	"log/slog"

	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/trace"
)

const (
	outcomeLoopErrored       = "loop_errored"
	outcomeParkFailed        = "park_failed"
	outcomeLoopDidNothing    = "loop_did_nothing"
	outcomeNothingToChange   = "nothing_to_change"
	outcomeCorrectionRefused = "correction_refused"
	outcomeNoCandidates      = "no_candidates"
	outcomeHelpShown         = "help_shown"
	outcomeUnclear           = "unclear"
)

func routerOutcome(intent orchestrator.Intent) string {
	switch intent {
	case orchestrator.IntentCreate, orchestrator.IntentUpdate, orchestrator.IntentDelete, orchestrator.IntentQuery, orchestrator.IntentCreateCategory, orchestrator.IntentAccountManage, orchestrator.IntentCategoryManage:
		return outcomePending
	case orchestrator.IntentReminderSet:
		return outcomeReminderSetRouted
	case orchestrator.IntentHelp:
		return outcomeHelpShown
	case orchestrator.IntentUnclear:
		return outcomeUnclear
	default:
		return outcomePending
	}
}

const (
	outcomePending           = "pending"
	outcomeReminderSetRouted = "reminder_set_routed"
)

func (l *Loop) logIntent(ctx context.Context, userID uint64, rawMessage string, intent orchestrator.Intent, runErr error) {
	if l.metrics == nil {
		return
	}
	if err := l.metrics.Log(userID, trace.ID(ctx), rawMessage, string(intent), false, initialOutcome(intent, runErr)); err != nil {
		slog.ErrorContext(ctx, "metric log intent failed", "err", err)
	}
}

func initialOutcome(intent orchestrator.Intent, runErr error) string {
	if runErr != nil {
		return outcomePending
	}
	return routerOutcome(intent)
}

func intentForExecutor(ex *agentExecutor, runErr error) orchestrator.Intent {
	var rateLimited *orchestrator.RateLimitedError
	switch {
	case errors.As(runErr, &rateLimited):
		return orchestrator.IntentQueued
	case runErr != nil:
		return orchestrator.IntentUnclear
	case ex.answerQuery:
		return orchestrator.IntentQuery
	case ex.settingsArea == SettingsAreaAccount:
		return orchestrator.IntentAccountManage
	case ex.settingsArea == SettingsAreaCategory:
		return orchestrator.IntentCreateCategory
	case ex.settingsArea == SettingsAreaCategoryManage:
		return orchestrator.IntentCategoryManage
	case ex.settingsArea == SettingsAreaReminder:
		return orchestrator.IntentReminderSet
	case len(ex.inserted) > 0:
		return orchestrator.IntentCreate
	case ex.reply == msgHelp:
		return orchestrator.IntentHelp
	case ex.reply == MsgAskRewrite:
		return orchestrator.IntentUnclear
	case len(ex.parked) > 0:
		switch ex.parked[0].Tool {
		case orchestrator.ToolDeleteMovements:
			return orchestrator.IntentDelete
		case orchestrator.ToolRecordMovements:
			return orchestrator.IntentCreate
		default:
			return orchestrator.IntentUpdate
		}
	default:
		return orchestrator.IntentUnclear
	}
}

func collectMovementIDs(ms []movement.Movement) []uint {
	return flow.CollectMovementIDs(ms)
}

func (l *Loop) setQueuedIntent(ctx context.Context, userID uint64, intent orchestrator.Intent) {
	if l.metrics == nil {
		return
	}
	if err := l.metrics.SetIntentIfQueued(userID, string(intent)); err != nil {
		slog.ErrorContext(ctx, "metric set queued intent failed", "err", err)
	}
}
