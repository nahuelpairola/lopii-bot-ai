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

	// AskOptionPrefix marca los callbacks de los botones aceleradores. Llevan el
	// ÍNDICE de la opción y no su texto porque callback_data son 64 bytes: un
	// nombre de categoría largo los desborda.
	AskOptionPrefix = "ask_opt:"
)

// NewAskUserFlow es UN flujo, de UN paso, que se pregunta a sí mismo.
//
// Reemplaza a los diez pasos a medida que había uno por hueco (cuenta,
// categoría, moneda, ...): las preguntas no se conocen al registrar el flujo,
// vienen en Data — que es exactamente lo que compran PromptText,
// EscapeOptionsFunc y SkipIf siendo func(data).
//
// El texto libre es EL punto, no un plan B. Preguntado "¿en qué categoría?", el
// usuario puede contestar una que no existe, y se crea. Ese es el callejón sin
// salida que cerramos: el picker viejo ofrecía sólo lo que ya existía, más
// Cancelar. Los botones aceleran; nunca encierran.
func NewAskUserFlow() *conversation.Flow {
	steps := map[string]conversation.Step{
		stepAsk: conversation.TextStep{
			PromptText: func(data conversation.Data) string {
				q, ok := NextOpenQuestion(data)
				if !ok {
					return "" // inalcanzable: SkipIf termina el flujo antes
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
			// OnText es por lo que el hook existe: la respuesta hay que ANOTARLA
			// contra su pregunta y sacarla de la cola en cada vuelta. Guardada bajo
			// una sola DataKey, la siguiente respuesta pisaría a la anterior.
			OnText: func(text string, data conversation.Data) conversation.Data {
				return AnswerCurrentQuestion(data, text)
			},
			SkipIf: func(data conversation.Data) (string, bool) {
				if conversation.Flag(data, conversation.KeyAskDiscarded) {
					return "", true
				}
				if _, ok := NextOpenQuestion(data); !ok {
					return "", true // no queda nada — el flujo terminó
				}
				return "", false
			},
			NextStep: stepAsk, // el auto-loop
		},
	}

	flow, err := conversation.NewFlow(AskUserFlowName, stepAsk, steps)
	if err != nil {
		panic(err)
	}
	return flow
}

// AnswerCurrentQuestion anota la respuesta contra la primera pregunta abierta,
// tira el texto crudo y descuenta una unidad de presupuesto.
//
// El presupuesto se gasta por VUELTA, no por pregunta: una respuesta puede ser
// inservible ("no sé") y hacer que el ejecutor vuelva a parkear con una pregunta
// nueva. Por eso el techo se congela al parkear (ver pendingaction.PendingAction)
// — si no, una lista de preguntas que crece se subiría su propio techo.
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
		// Se acabó el techo con preguntas todavía abiertas: la acción se tira
		// ENTERA. Quien drena es el que le avisa al usuario qué se cayó — tirar
		// un movimiento en silencio es justo la falla que esto viene a evitar.
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

// EncodeOpenQuestions guarda las preguntas como UN string JSON en vez de como
// []interface{}: Data va y vuelve de una columna JSONB, y un string sobrevive
// ese viaje sin que haya que desarmar map[string]interface{} a mano.
// json.Marshal de puros strings no puede fallar.
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
