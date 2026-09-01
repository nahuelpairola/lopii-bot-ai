package flow

import (
	"encoding/json"
	"strconv"
	"strings"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/pendingaction"
)

const (
	stepAsk = "ask"

	AskOptionPrefix = "ask_opt:"
)

func NewAskUserFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepAsk: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				q, ok := NextOpenQuestion(data)
				if !ok {
					return ""
				}
				return q.Prompt
			},
			DataKey: conversation.KeyAskRawAnswer,
			EscapeOptionsFunc: func(data conversation.Data) []conversation.ChoiceOption {
				q, ok := NextOpenQuestion(data)
				if !ok {
					return nil
				}
				opts := make([]conversation.ChoiceOption, 0, len(q.Options)+1)
				for i, o := range q.Options {
					opts = append(opts, conversation.ChoiceOption{
						Label:    o,
						Value:    AskOptionPrefix + strconv.Itoa(i),
						NextStep: stepAsk,
					})
				}
				return append(opts, CancelOption)
			},
			OnEscape: func(value string, data conversation.Data) conversation.Data {
				if value == OptionCancel {
					next := conversation.CopyData(data)
					conversation.SetFlag(next, conversation.KeyCancelled)
					return next
				}
				q, ok := NextOpenQuestion(data)
				if !ok {
					return data
				}
				i, err := strconv.Atoi(strings.TrimPrefix(value, AskOptionPrefix))
				if err != nil || i < 0 || i >= len(q.Options) {
					return data
				}
				return AnswerCurrentQuestion(data, q.Options[i])
			},
			OnText: func(text string, data conversation.Data) conversation.Data {
				return AnswerCurrentQuestion(data, text)
			},
			SkipIf: func(data conversation.Data) (string, bool) {
				if conversation.Flag(data, conversation.KeyAskDiscarded) {
					return "", true
				}
				if _, ok := NextOpenQuestion(data); !ok {
					return "", true
				}
				return "", false
			},
			NextStep: stepAsk,
		},
	}

	flow, err := conversation.NewFlow(AskUserFlowName, stepAsk, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

func AnswerCurrentQuestion(data conversation.Data, answer string) conversation.Data {
	next := conversation.CopyData(data)
	delete(next, conversation.KeyAskRawAnswer)

	questions := DecodeOpenQuestions(next)
	for i := range questions {
		if questions[i].Answer == "" {
			questions[i].Answer = answer
			break
		}
	}
	next[conversation.KeyOpenQuestions] = EncodeOpenQuestions(questions)

	budget := AskBudget(next) - 1
	next[conversation.KeyAskBudget] = strconv.Itoa(budget)
	if budget <= 0 && HasOpenQuestion(questions) {
		conversation.SetFlag(next, conversation.KeyAskDiscarded)
	}
	return next
}

func NextOpenQuestion(data conversation.Data) (pendingaction.OpenQuestion, bool) {
	for _, q := range DecodeOpenQuestions(data) {
		if q.Answer == "" {
			return q, true
		}
	}
	return pendingaction.OpenQuestion{}, false
}

func HasOpenQuestion(questions []pendingaction.OpenQuestion) bool {
	for _, q := range questions {
		if q.Answer == "" {
			return true
		}
	}
	return false
}

func EncodeOpenQuestions(questions []pendingaction.OpenQuestion) string {
	encoded, _ := json.Marshal(questions)
	return string(encoded)
}

func DecodeOpenQuestions(data conversation.Data) []pendingaction.OpenQuestion {
	var questions []pendingaction.OpenQuestion
	_ = json.Unmarshal([]byte(conversation.StringOrEmpty(data[conversation.KeyOpenQuestions])), &questions)
	return questions
}

func AskBudget(data conversation.Data) int {
	n, _ := strconv.Atoi(conversation.StringOrEmpty(data[conversation.KeyAskBudget]))
	return n
}
