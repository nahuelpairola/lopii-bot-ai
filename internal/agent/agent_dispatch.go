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
	"lopiibot.com/internal/orchestrator"
	"lopiibot.com/internal/pendingaction"
	"lopiibot.com/internal/trace"
)

const budgetSlack = 2

func parkAgentActions(ctx context.Context, svc agentServices, userID uint64, actions []parkedAction) error {
	for i, a := range actions {
		payload, err := json.Marshal(a.Payload)
		if err != nil {
			return fmt.Errorf("park %s: payload: %w", a.Tool, err)
		}
		questions, err := json.Marshal(a.Questions)
		if err != nil {
			return fmt.Errorf("park %s: questions: %w", a.Tool, err)
		}
		row := &pendingaction.PendingAction{
			UserID:    userID,
			Tool:      a.Tool,
			Payload:   payload,
			Questions: questions,
			Budget:    len(a.Questions) + budgetSlack,
			Position:  i,
			TraceID:   trace.ID(ctx),
		}
		if err := svc.ActionsInsert(row); err != nil {
			return fmt.Errorf("park %s: %w", a.Tool, err)
		}
	}
	return nil
}

func drainNextAgentAction(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64) error {
	if !svc.ActionsEnabled() {
		return nil
	}
	action, err := svc.ActionsNextForUser(userID)
	if errors.Is(err, pendingaction.ErrNoPendingAction) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("drain: next action: %w", err)
	}

	var questions []pendingaction.OpenQuestion
	if err := json.Unmarshal(action.Questions, &questions); err != nil {
		return fmt.Errorf("drain: questions: %w", err)
	}
	if flow.HasOpenQuestion(questions) {
		return openAskUser(ctx, svc, chat, userID, action, action.Budget)
	}
	return resumeAgentAction(ctx, svc, chat, userID, action)
}

func openAskUser(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, action *pendingaction.PendingAction, budget int) error {
	if budget <= 0 {
		return discardAgentAction(ctx, svc, chat, userID, action)
	}
	seed := conversation.Data{
		conversation.KeyActionID:      strconv.FormatUint(action.ID, 10),
		conversation.KeyOpenQuestions: string(action.Questions),
		conversation.KeyAskBudget:     strconv.Itoa(budget),
	}
	prompt, err := svc.EngineStartWithData(userID, flow.AskUserFlowName, seed)
	if err != nil {
		return fmt.Errorf("drain: start ask_user: %w", err)
	}
	svc.SendPrompt(ctx, chat, prompt)
	return nil
}

func finishAskUserFlow(ctx context.Context, svc agentServices, chat messenger.Chat, data conversation.Data) {
	userID := data.UserID()
	action, err := openAction(svc, userID, data)
	if err != nil {
		slog.ErrorContext(ctx, "ask_user finished with no matching action", "err", err, "user_id", userID)
		svc.SendText(ctx, chat, flow.MsgSomethingBroke)
		return
	}

	if conversation.Flag(data, conversation.KeyCancelled) {
		outcome, msg := flow.OutcomeUpdateCancelled, flow.MsgUpdateCancelled
		if action.Tool == orchestrator.ToolDeleteMovements {
			outcome, msg = flow.OutcomeDeleteCancelled, flow.MsgDeleteCancelled
		}
		resolveMetric(ctx, svc, userID, outcome)
		dropAgentAction(ctx, svc, chat, userID, action, msg)
		return
	}
	if conversation.Flag(data, conversation.KeyAskDiscarded) {
		if err := discardAgentAction(ctx, svc, chat, userID, action); err != nil {
			slog.ErrorContext(ctx, "discard parked action failed", "err", err)
		}
		return
	}

	answers := flow.DecodeOpenQuestions(data)
	payload, resolved := applyAnswers(action, answers)
	if !resolved {
		if err := researchCandidates(svc, userID, action, answers); err != nil {
			slog.ErrorContext(ctx, "re-search candidates failed", "err", err, "user_id", userID)
		}
		if err := openAskUser(ctx, svc, chat, userID, action, flow.AskBudget(data)); err != nil {
			slog.ErrorContext(ctx, "reopen ask_user failed", "err", err)
			svc.SendText(ctx, chat, flow.MsgSomethingBroke)
		}
		return
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		slog.ErrorContext(ctx, "marshal resolved payload failed", "err", err)
		svc.SendText(ctx, chat, flow.MsgSomethingBroke)
		return
	}
	action.Payload = raw
	if err := resumeAgentAction(ctx, svc, chat, userID, action); err != nil {
		slog.ErrorContext(ctx, "resume parked action failed", "err", err, "tool", action.Tool)
		svc.SendText(ctx, chat, flow.MsgSomethingBroke)
	}
}

func openAction(svc agentServices, userID uint64, data conversation.Data) (*pendingaction.PendingAction, error) {
	if !svc.ActionsEnabled() {
		return nil, errors.New("no pending action repository")
	}
	action, err := svc.ActionsNextForUser(userID)
	if err != nil {
		return nil, err
	}
	if strconv.FormatUint(action.ID, 10) != conversation.StringOrEmpty(data[conversation.KeyActionID]) {
		return nil, fmt.Errorf("ask_user was answering action %s, queue head is %d", conversation.StringOrEmpty(data[conversation.KeyActionID]), action.ID)
	}
	return action, nil
}

func applyAnswers(action *pendingaction.PendingAction, answers []pendingaction.OpenQuestion) (agentPayload, bool) {
	var payload agentPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return payload, false
	}
	for _, q := range answers {
		switch q.Key {
		case questionKeyCandidate:
			idx := indexOf(q.Options, q.Answer)
			if idx < 0 {
				return payload, false
			}
			payload.Chosen = idx
		case questionKeyChange:
			payload.Change = strings.TrimSpace(payload.Change + " " + q.Answer)
			payload.ChangeAnswer = q.Answer
			if indexOf(q.Options, q.Answer) >= 0 {
				payload.PickedChangeField = true
				payload.PickedField = string(changeFieldForLabel(q.Answer))
			} else {
				payload.GaveChangeValue = true
				if payload.PickedField != "" {
					payload.Changes = []correctionChange{{
						Field: changeField(payload.PickedField), Op: opSet, Value: q.Answer,
					}}
				}
			}
		}
	}
	return payload, payload.Chosen >= 0
}

func researchCandidates(svc agentServices, userID uint64, action *pendingaction.PendingAction, answers []pendingaction.OpenQuestion) error {
	var payload agentPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return fmt.Errorf("research: payload: %w", err)
	}
	answer := ""
	for _, q := range answers {
		if q.Key == questionKeyCandidate && q.Answer != "" {
			answer = q.Answer
		}
	}
	if answer == "" || payload.SearchText == "" {
		return nil
	}

	searchText := payload.SearchText + " " + answer
	groups, err := resolveCandidates(svc, userID, searchText, payload.DateFrom, payload.DateTo)
	if err != nil {
		return fmt.Errorf("research: resolve: %w", err)
	}
	if len(groups) == 0 {
		return nil
	}

	candidates := make([]flow.CandidateGroup, 0, len(groups))
	options := make([]string, 0, len(groups))
	for _, g := range groups {
		candidates = append(candidates, toCandidateGroup(g))
		options = append(options, candidateLabel(g))
	}
	payload.Candidates = candidates
	payload.Chosen = -1

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("research: marshal payload: %w", err)
	}

	question := flow.MsgPickUpdateCandidate(nil)
	if action.Tool == orchestrator.ToolDeleteMovements {
		question = flow.MsgPickDeleteCandidate(nil)
	}
	if !matchesMessage(groups[0], searchText) {
		question = flow.MsgPickRecentFallback
	}
	rawQuestions, err := json.Marshal([]pendingaction.OpenQuestion{{
		Key: questionKeyCandidate, Prompt: question + " " + flow.MsgCanRetypeToSearch, Options: options,
	}})
	if err != nil {
		return fmt.Errorf("research: marshal questions: %w", err)
	}

	action.Payload = rawPayload
	action.Questions = rawQuestions
	return svc.ActionsUpdate(action)
}

func indexOf(options []string, want string) int {
	for i, o := range options {
		if o == want {
			return i
		}
	}
	return -1
}

func resumeAgentAction(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, action *pendingaction.PendingAction) error {
	var payload agentPayload
	if err := json.Unmarshal(action.Payload, &payload); err != nil {
		return fmt.Errorf("resume: payload: %w", err)
	}

	var chosen flow.CandidateGroup
	if action.Tool != orchestrator.ToolRecordMovements && !isBatchCorrection(payload) {
		var err error
		if chosen, err = chosenCandidate(payload); err != nil {
			return err
		}
	}

	if err := svc.ActionsDelete(action.ID); err != nil {
		return fmt.Errorf("resume: delete action: %w", err)
	}

	switch action.Tool {
	case orchestrator.ToolRecordMovements:
		seed := conversation.Data{}
		for k, v := range payload.Seed {
			seed[k] = v
		}
		flowName := flow.MovementCreateFlowName
		if _, gated := payload.Seed[conversation.KeyGatePrompt]; gated {
			flowName = flow.MovementNegativeConfirmFlowName
		}
		return svc.StartFlow(ctx, chat, userID, flowName, seed, "drain: start "+flowName)
	case orchestrator.ToolCorrectMovement:
		if len(payload.Changes) > 0 {
			groups := []flow.CandidateGroup{chosen}
			if isBatchCorrection(payload) {
				groups = payload.Candidates
			}
			return applyStructuredCorrection(ctx, svc, chat, userID, payload, groups)
		}
		if len(payload.Changes) == 0 && !payload.GaveChangeValue {
			return parkChangeQuestion(ctx, svc, chat, userID, payload.Change, chosen.TransactionID, chosen.OldIDs, chosen.Rows,
				ChangeAsk{pickedField: payload.PickedChangeField, gaveValue: payload.GaveChangeValue, answer: payload.ChangeAnswer, field: payload.PickedField})
		}
		return proceedToUpdateConfirm(ctx, svc, chat, userID, payload.Change, chosen.TransactionID, chosen.OldIDs, chosen.Rows, ChangeAsk{pickedField: payload.PickedChangeField, gaveValue: payload.GaveChangeValue, answer: payload.ChangeAnswer, field: payload.PickedField})
	case orchestrator.ToolDeleteMovements:
		seed := conversation.Data{
			conversation.KeyCandidateGroups: encodeCandidateGroupList([]flow.CandidateGroup{chosen}),
			conversation.KeyResolvedIndex:   "0",
		}
		return svc.StartFlow(ctx, chat, userID, flow.MovementDeleteFlowName, seed, "drain: start movement_delete flow")
	default:
		return fmt.Errorf("resume: tool %q has no resume path", action.Tool)
	}
}

func chosenCandidate(payload agentPayload) (flow.CandidateGroup, error) {
	if payload.Chosen < 0 || payload.Chosen >= len(payload.Candidates) {
		return flow.CandidateGroup{}, fmt.Errorf("resume: candidate %d out of range (%d)", payload.Chosen, len(payload.Candidates))
	}
	return payload.Candidates[payload.Chosen], nil
}

func discardAgentAction(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, action *pendingaction.PendingAction) error {
	slog.InfoContext(ctx, "parked action discarded: budget exhausted",
		"tool", action.Tool, "user_id", userID, "budget", action.Budget)
	resolveMetric(ctx, svc, userID, flow.OutcomeCreateFailed)
	dropAgentAction(ctx, svc, chat, userID, action, msgAgentActionDiscarded(describeAction(action)))
	return nil
}

func dropAgentAction(ctx context.Context, svc agentServices, chat messenger.Chat, userID uint64, action *pendingaction.PendingAction, message string) {
	if err := svc.ActionsDelete(action.ID); err != nil {
		slog.ErrorContext(ctx, "delete parked action failed", "err", err)
	}
	svc.SendText(ctx, chat, message)
	if err := drainNextAgentAction(ctx, svc, chat, userID); err != nil {
		slog.ErrorContext(ctx, "drain after drop failed", "err", err)
	}
}

func msgAgentActionDiscarded(what string) string {
	return fmt.Sprintf("No terminé de entender %s, así que lo dejo sin hacer.\n\nSi querés, escribímelo de nuevo con un poco más de detalle.", what)
}

func describeAction(action *pendingaction.PendingAction) string {
	var payload agentPayload
	_ = json.Unmarshal(action.Payload, &payload)
	if payload.Change != "" {
		return "«" + payload.Change + "»"
	}
	if action.Tool == orchestrator.ToolDeleteMovements {
		return "el borrado que me pediste"
	}
	return "lo que me pediste"
}
