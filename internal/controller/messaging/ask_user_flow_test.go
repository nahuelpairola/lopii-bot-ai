package messaging

import (
	"strconv"
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
	"lopiibot.com/internal/pendingaction"
)

func newAskUserTestEngine(t *testing.T) *conversation.Engine {
	t.Helper()
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(flow.NewAskUserFlow())
	return engine
}

func askSeed(t *testing.T, budget int, questions ...pendingaction.OpenQuestion) conversation.Data {
	t.Helper()
	return conversation.Data{
		conversation.KeyActionID:      "7",
		conversation.KeyOpenQuestions: flow.EncodeOpenQuestions(questions),
		conversation.KeyAskBudget:     strconv.Itoa(budget),
	}
}

func TestAskUser_BothAnswersSurvive(t *testing.T) {
	engine := newAskUserTestEngine(t)
	seed := askSeed(t, 4,
		pendingaction.OpenQuestion{Key: "row0.account", Prompt: "¿De qué cuenta salió?"},
		pendingaction.OpenQuestion{Key: "row0.category", Prompt: "¿En qué categoría lo pongo?"},
	)

	prompt, err := engine.StartWithData(1, flow.AskUserFlowName, seed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.Text, "¿De qué cuenta salió?") {
		t.Fatalf("la primera pregunta no es la que se muestra:\n%s", prompt.Text)
	}

	res, found, err := engine.Handle(1, conversation.Input{Text: "Brubank"})
	if err != nil || !found {
		t.Fatalf("primera respuesta: found=%v err=%v", found, err)
	}
	if res.Finished {
		t.Fatal("el flujo terminó con una pregunta todavía abierta")
	}
	if !strings.Contains(res.Prompt.Text, "¿En qué categoría lo pongo?") {
		t.Fatalf("no avanzó a la segunda pregunta:\n%s", res.Prompt.Text)
	}

	res, _, err = engine.Handle(1, conversation.Input{Text: "Comida"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Finished {
		t.Fatal("sin preguntas abiertas el flujo tiene que terminar")
	}

	answered := flow.DecodeOpenQuestions(res.Data)
	if len(answered) != 2 {
		t.Fatalf("want 2 questions back, got %d: %+v", len(answered), answered)
	}
	if answered[0].Answer != "Brubank" {
		t.Errorf("la PRIMERA respuesta se perdió: %+v", answered)
	}
	if answered[1].Answer != "Comida" {
		t.Errorf("la segunda respuesta se perdió: %+v", answered)
	}
}

func TestAskUser_ButtonAndFreeTextLandInTheSamePlace(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input conversation.Input
		want  string
	}{
		{"botón", conversation.Input{CallbackData: flow.AskOptionPrefix + "1"}, "Efectivo"},
		{"texto libre", conversation.Input{Text: "Una cuenta que no existe"}, "Una cuenta que no existe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newAskUserTestEngine(t)
			seed := askSeed(t, 3, pendingaction.OpenQuestion{
				Key:     "row0.account",
				Prompt:  "¿De qué cuenta salió?",
				Options: []string{"Mercado Pago", "Efectivo"},
			})
			if _, err := engine.StartWithData(1, flow.AskUserFlowName, seed); err != nil {
				t.Fatal(err)
			}
			res, _, err := engine.Handle(1, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if !res.Finished {
				t.Fatal("con una sola pregunta contestada el flujo termina")
			}
			got := flow.DecodeOpenQuestions(res.Data)
			if len(got) != 1 || got[0].Answer != tc.want {
				t.Errorf("want answer %q, got %+v", tc.want, got)
			}
		})
	}
}

func TestAskUser_OffersTheOptionsAsButtonsPlusCancel(t *testing.T) {
	engine := newAskUserTestEngine(t)
	seed := askSeed(t, 3, pendingaction.OpenQuestion{
		Key: "row0.account", Prompt: "¿De qué cuenta salió?",
		Options: []string{"Mercado Pago", "Efectivo"},
	})
	prompt, err := engine.StartWithData(1, flow.AskUserFlowName, seed)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt.Buttons) != 3 {
		t.Fatalf("want 2 opciones + Cancelar, got %d: %+v", len(prompt.Buttons), prompt.Buttons)
	}
	if prompt.Buttons[0].Label != "Mercado Pago" || prompt.Buttons[0].Data != flow.AskOptionPrefix+"0" {
		t.Errorf("el primer botón no es la primera opción: %+v", prompt.Buttons[0])
	}
	if prompt.Buttons[1].Data != flow.AskOptionPrefix+"1" {
		t.Errorf("el callback tiene que ser el índice: %+v", prompt.Buttons[1])
	}
	if prompt.Buttons[2].Data != flow.OptionCancel {
		t.Errorf("falta el Cancelar: %+v", prompt.Buttons[2])
	}
}

func TestAskUser_CancelMarksCancelled(t *testing.T) {
	engine := newAskUserTestEngine(t)
	seed := askSeed(t, 3, pendingaction.OpenQuestion{Key: "row0.account", Prompt: "¿De qué cuenta salió?"})
	if _, err := engine.StartWithData(1, flow.AskUserFlowName, seed); err != nil {
		t.Fatal(err)
	}
	res, _, err := engine.Handle(1, conversation.Input{CallbackData: flow.OptionCancel})
	if err != nil || !res.Finished {
		t.Fatalf("cancel: res=%+v err=%v", res, err)
	}
	if !conversation.Flag(res.Data, conversation.KeyCancelled) {
		t.Errorf("cancelar tiene que dejar la marca: %+v", res.Data)
	}
	if conversation.Flag(res.Data, conversation.KeyAskDiscarded) {
		t.Errorf("cancelar NO es lo mismo que quedarse sin presupuesto: %+v", res.Data)
	}
}

func TestAskUser_SkipsWhenNothingIsOpen(t *testing.T) {
	engine := newAskUserTestEngine(t)
	seed := askSeed(t, 3, pendingaction.OpenQuestion{
		Key: "row0.account", Prompt: "¿De qué cuenta salió?", Answer: "Brubank",
	})
	if _, err := engine.StartWithData(1, flow.AskUserFlowName, seed); err == nil {
		t.Fatal("sin preguntas abiertas StartWithData tiene que fallar, no mostrar un prompt vacío")
	}
}

func TestAskUser_BudgetExhaustedDiscardsWhole(t *testing.T) {
	engine := newAskUserTestEngine(t)
	seed := askSeed(t, 1,
		pendingaction.OpenQuestion{Key: "row0.account", Prompt: "¿De qué cuenta salió?"},
		pendingaction.OpenQuestion{Key: "row0.category", Prompt: "¿En qué categoría lo pongo?"},
	)
	if _, err := engine.StartWithData(1, flow.AskUserFlowName, seed); err != nil {
		t.Fatal(err)
	}
	res, _, err := engine.Handle(1, conversation.Input{Text: "Brubank"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Finished {
		t.Fatal("agotado el presupuesto el flujo termina, no sigue preguntando")
	}
	if !conversation.Flag(res.Data, conversation.KeyAskDiscarded) {
		t.Errorf("falta la marca de descarte: %+v", res.Data)
	}
	if conversation.Flag(res.Data, conversation.KeyCancelled) {
		t.Errorf("quedarse sin presupuesto NO es que el usuario cancele: %+v", res.Data)
	}
	got := flow.DecodeOpenQuestions(res.Data)
	if len(got) != 2 || got[0].Answer != "Brubank" || got[1].Answer != "" {
		t.Errorf("el estado de las preguntas no sobrevivió al descarte: %+v", got)
	}
}
