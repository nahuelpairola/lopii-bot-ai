package messaging

import (
	"context"
	"log/slog"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

const (
	outcomeLoopErrored         = "loop_errored"
	outcomeParkFailed          = "park_failed"
	outcomeLoopDidNothing      = "loop_did_nothing"
	outcomeNoCandidates        = "no_candidates"
	outcomeQueryAnswered       = "query_answered"
	outcomeQueryFailed         = "query_failed"
	outcomeAccountCreateRouted = flow.OutcomeAccountCreateRouted
	outcomeReminderSetRouted   = "reminder_set_routed"
	outcomeHelpShown           = "help_shown"
	outcomeUnclear             = "unclear"
	outcomeCategoryMatchUsed   = flow.OutcomeCategoryMatchUsed
	outcomeCategoryCreated     = flow.OutcomeCategoryCreated
	outcomeCategoryCancelled   = flow.OutcomeCategoryCancelled

	outcomeAccountRenamed         = flow.OutcomeAccountRenamed
	outcomeAccountAdjusted        = flow.OutcomeAccountAdjusted
	outcomeAccountDefaultSet      = flow.OutcomeAccountDefaultSet
	outcomeAccountManageCancelled = flow.OutcomeAccountManageCancelled
)

const (
	outcomeCreateInserted  = flow.OutcomeCreateInserted
	outcomeCreateCancelled = flow.OutcomeCreateCancelled
	outcomeCreateFailed    = flow.OutcomeCreateFailed
	outcomeUpdateConfirmed = flow.OutcomeUpdateConfirmed
	outcomeUpdateCancelled = flow.OutcomeUpdateCancelled
	outcomeWriteFailed     = flow.OutcomeWriteFailed
	outcomeDeleteConfirmed = flow.OutcomeDeleteConfirmed
	outcomeDeleteCancelled = flow.OutcomeDeleteCancelled
)

func writeOutcomeFor(data conversation.Data) string {
	return flow.WriteOutcomeFor(data)
}

func failureOutcomeFor(data conversation.Data) string {
	return flow.FailureOutcomeFor(data)
}

func (c *controller) resolveMetric(ctx context.Context, userID uint64, outcome string, movementIDs ...uint) {
	if c.metrics == nil {
		return
	}
	if err := c.metrics.Resolve(userID, outcome, movementIDs); err != nil {
		slog.ErrorContext(ctx, "metric resolve failed", "outcome", outcome, "err", err)
	}
}
