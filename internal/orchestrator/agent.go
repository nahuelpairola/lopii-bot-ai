package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// maxAgentIterations caps how many tool rounds Run executes before forcing a
// narration. Higher than AnswerQuery's 3 because the unified loop legitimately
// chains more: a compound message can read, write and park in one turn. Still a
// runaway guard, not the expected path — the common turn is one round.
const maxAgentIterations = 5

var ErrAgentMaxIterations = errors.New("orchestrator: agent loop exceeded max iterations")

// kindRank orders a round's calls: every write, then every read, then every
// parking. See orderCallsByKind.
func kindRank(k AgentToolKind) int {
	switch k {
	case KindWrite:
		return 0
	case KindAction:
		return 2
	default:
		// KindRead and anything undeclared. Read is the conservative slot: it
		// neither jumps ahead of a write nor gets deferred past one.
		return 1
	}
}

// orderCallsByKind returns the round's calls sorted write → read → action,
// preserving the model's relative order within each class.
//
// This is load-bearing, not tidiness. Under batching a sum_movements and the
// record_movements it must count arrive in the SAME round, and the order the
// model happened to list them in is not something to rely on. Running the read
// first reports a total that excludes the rows about to be inserted — wrong
// money, and silently wrong.
//
// Reordering is free at the protocol level because results map back by
// tool_call_id, not position; and it is safe semantically because within one
// round the model chose every call before seeing any result.
func orderCallsByKind(calls []loopToolCall, tools []AgentTool) []loopToolCall {
	kindOf := make(map[string]AgentToolKind, len(tools))
	for _, t := range tools {
		kindOf[t.Name] = t.Kind
	}
	ordered := make([]loopToolCall, len(calls))
	copy(ordered, calls)
	sort.SliceStable(ordered, func(i, j int) bool {
		return kindRank(kindOf[ordered[i].Function.Name]) < kindRank(kindOf[ordered[j].Function.Name])
	})
	return ordered
}

// Run drives the unified agent loop: it sends the tools, executes every call a
// round emits via the caller's execute closure (scoped to the user), feeds each
// result back as a role:"tool" message, and repeats until the model narrates.
//
// It returns the narration to send to the user. What was written or parked is
// the caller's business — execute is the caller's closure, so it already knows.
//
// Three things differ from AnswerQuery, which stays untouched until stage 4:
//
//   - The turn cut: an assistant message carrying content ALONGSIDE tool_calls
//     means the model already said what it had to. The calls still run — their
//     side effects are wanted — and then the turn ends without spending another
//     round replaying the whole prefix.
//   - Class order inside a round: orderCallsByKind, above.
//   - A cap of 5 rounds instead of 3.
//
// A tool executor error is fed back to the model as text, not aborted, so it
// can recover or explain.
func (o *Orchestrator) Run(ctx context.Context, systemPrompt, userText string, history []QueryTurn, tools []AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
	toolDefs := make([]toolDef, len(tools))
	for i, t := range tools {
		toolDefs[i] = toolDef{
			Type:     "function",
			Function: toolFunction{Name: t.Name, Description: t.Description, Parameters: t.Parameters},
		}
	}

	messages := []loopMessage{{Role: "system", Content: systemPrompt}}
	for _, t := range history {
		messages = append(messages,
			loopMessage{Role: "user", Content: t.Question},
			loopMessage{Role: "assistant", Content: t.Answer},
		)
	}
	messages = append(messages, loopMessage{Role: "user", Content: userText})

	for i := 0; i < maxAgentIterations; i++ {
		// Round 0 forces a call. Opening in "auto" risks the worst failure
		// mode: the model replying "listo, anoté tus $5.000" without ever
		// calling record_movements — silent data loss. reply_help and
		// ask_rewrite exist so every message has something to call.
		choice := "auto"
		if i == 0 {
			choice = "required"
		}
		assistant, err := o.client.chatCompletionLoop(ctx, callTypeQuery, o.agentModel, messages, toolDefs, choice)
		if errors.Is(err, ErrNothingToExtract) && choice == "required" {
			// The model refused to call anything under tool_choice:"required",
			// and Groq turns that into a hard 400. Observed on real correction
			// messages ("Le erre eran 1500"): with 15 tools and a short,
			// referent-less message, gpt-oss-20b emits nothing at all.
			//
			// ask_rewrite exists precisely so every message has something to
			// call, but the model does not always reach for it. Rather than
			// fail the turn, ask once more in "auto": a turn may legitimately
			// end in a narrated question, which is the design's own escape.
			//
			// Silent data loss is not reopened by this. The model already
			// declined to call a tool, so there was nothing to record; the risk
			// it now claims to have recorded something is what the prompt's
			// "no repitas el detalle" rule and an empty receipt guard against.
			assistant, err = o.client.chatCompletionLoop(ctx, callTypeQuery, o.agentModel, messages, toolDefs, "auto")
		}
		if err != nil {
			return "", fmt.Errorf("orchestrator: agent run: %w", err)
		}

		if len(assistant.ToolCalls) == 0 {
			return assistant.Content, nil
		}

		messages = append(messages, assistant)
		for _, call := range orderCallsByKind(assistant.ToolCalls, tools) {
			result, execErr := execute(call.Function.Name, json.RawMessage(call.Function.Arguments))
			if execErr != nil {
				result = fmt.Sprintf("error: %v", execErr)
			}
			messages = append(messages, loopMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
		}

		// The cut. Checked AFTER executing, so the writes and parkings the
		// model asked for still happen — it narrated and acted in one message.
		if strings.TrimSpace(assistant.Content) != "" {
			return assistant.Content, nil
		}
	}

	// Cap reached while the model still wanted tools. Force one narration from
	// the results already gathered. Tools are omitted (nil, not toolDefs):
	// Groq 400s hard if the model attempts a call while tool_choice is "none".
	final, err := o.client.chatCompletionLoop(ctx, callTypeQuery, o.agentModel, messages, nil, "none")
	if err != nil {
		return "", fmt.Errorf("orchestrator: agent run (final): %w", err)
	}
	if strings.TrimSpace(final.Content) == "" {
		return "", ErrAgentMaxIterations
	}
	return final.Content, nil
}
