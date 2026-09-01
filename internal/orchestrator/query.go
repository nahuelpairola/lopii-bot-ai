package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type AgentToolKind string

const (
	KindRead   AgentToolKind = "read"
	KindWrite  AgentToolKind = "write"
	KindAction AgentToolKind = "action"
)

type AgentTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Kind        AgentToolKind
	When        string
}

type QueryTurn struct {
	Question string
	Answer   string
}

const maxQueryIterations = 3

var ErrQueryMaxIterations = errors.New("orchestrator: query loop exceeded max iterations")

func (o *Orchestrator) queryChain() []string {
	return append([]string{o.queryModel}, o.queryFallbacks...)
}

func (o *Orchestrator) narrationChain() []string {
	if o.narrationModel == "" {
		return o.queryChain()
	}
	chain := []string{o.narrationModel}
	for _, m := range o.queryChain() {
		if m != o.narrationModel {
			chain = append(chain, m)
		}
	}
	return chain
}

func describeCall(name, args string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil || len(m) == 0 {
		return name
	}
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v == nil || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return name
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, m[k]))
	}
	return name + " con " + strings.Join(parts, ", ")
}

func (o *Orchestrator) AnswerQuery(ctx context.Context, systemPrompt, userText string, history []QueryTurn, tools []AgentTool, execute func(name string, args json.RawMessage) (string, error)) (string, error) {
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

	var toolResults []string

	for i := 0; i < maxQueryIterations; i++ {
		choice, capTokens := "auto", maxQueryCompletionTokens
		if i == 0 {
			choice, capTokens = "required", maxFirstRoundCompletionTokens
		}
		assistant, err := o.roundWithFallback(ctx, callTypeQuery, o.queryChain(), messages, toolDefs, choice, capTokens)
		if err != nil {
			return "", fmt.Errorf("orchestrator: answer query: %w", err)
		}
		if len(assistant.ToolCalls) == 0 {
			return assistant.Content, nil
		}
		messages = append(messages, assistant)
		for _, call := range assistant.ToolCalls {
			result, execErr := execute(call.Function.Name, json.RawMessage(call.Function.Arguments))
			if execErr != nil {
				result = fmt.Sprintf("error: %v", execErr)
			}
			messages = append(messages, loopMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    result,
			})
			toolResults = append(toolResults, describeCall(call.Function.Name, call.Function.Arguments)+" → "+result)
		}
	}

	finalMessages := []loopMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userText},
		{Role: "user", Content: "Datos que se juntaron:\n" + strings.Join(toolResults, "\n") +
			"\n\nOJO: están INCOMPLETOS, quedaron cosas sin averiguar. Redactá la respuesta final " +
			"para el usuario usando SÓLO lo que está acá arriba. De lo que te hayan preguntado y no " +
			"aparezca en esta lista, decí que no llegaste a averiguarlo: no afirmes que no existe, " +
			"que no hay, ni que dio cero."},
	}
	final, err := o.roundWithFallback(ctx, callTypeQuery, o.narrationChain(), finalMessages, nil, "none", maxNarrationCompletionTokens)
	if err != nil {
		return "", fmt.Errorf("orchestrator: answer query (final): %w", err)
	}
	if strings.TrimSpace(final.Content) == "" {
		return "", ErrQueryMaxIterations
	}
	return final.Content, nil
}
