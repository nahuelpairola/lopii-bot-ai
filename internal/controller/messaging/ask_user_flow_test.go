package messaging

import (
	"strconv"
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/pendingaction"
)

func newAskUserTestEngine(t *testing.T) *conversation.Engine {
	t.Helper()
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(NewAskUserFlow())
	return engine
}

func askSeed(t *testing.T, budget int, questions ...pendingaction.OpenQuestion) conversation.Data {
	t.Helper()
	return conversation.Data{
		keyActionID:      "7",
		keyOpenQuestions: encodeOpenQuestions(questions),
		keyAskBudget:     strconv.Itoa(budget),
	}
}

// TestAskUser_BothAnswersSurvive es LA regresión por la que existe OnText: un
// paso auto-recursivo que guardara la respuesta bajo una sola DataKey se
// pisaría a sí mismo y sólo sobreviviría la última.
func TestAskUser_BothAnswersSurvive(t *testing.T) {
	engine := newAskUserTestEngine(t)
	seed := askSeed(t, 4,
		pendingaction.OpenQuestion{Key: "row0.account", Prompt: "¿De qué cuenta salió?"},
		pendingaction.OpenQuestion{Key: "row0.category", Prompt: "¿En qué categoría lo pongo?"},
	)

	prompt, err := engine.StartWithData(1, askUserFlowName, seed)
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

	answered := decodeOpenQuestions(res.Data)
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

// TestAskUser_ButtonAndFreeTextLandInTheSamePlace: los botones aceleran, no
// encierran. Una respuesta escrita a mano vale exactamente igual que un tap.
func TestAskUser_ButtonAndFreeTextLandInTheSamePlace(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input conversation.Input
		want  string
	}{
		{"botón", conversation.Input{CallbackData: askOptionPrefix + "1"}, "Efectivo"},
		{"texto libre", conversation.Input{Text: "Una cuenta que no existe"}, "Una cuenta que no existe"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newAskUserTestEngine(t)
			seed := askSeed(t, 3, pendingaction.OpenQuestion{
				Key:     "row0.account",
				Prompt:  "¿De qué cuenta salió?",
				Options: []string{"Mercado Pago", "Efectivo"},
			})
			if _, err := engine.StartWithData(1, askUserFlowName, seed); err != nil {
				t.Fatal(err)
			}
			res, _, err := engine.Handle(1, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if !res.Finished {
				t.Fatal("con una sola pregunta contestada el flujo termina")
			}
			got := decodeOpenQuestions(res.Data)
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
	prompt, err := engine.StartWithData(1, askUserFlowName, seed)
	if err != nil {
		t.Fatal(err)
	}
	if len(prompt.Buttons) != 3 {
		t.Fatalf("want 2 opciones + Cancelar, got %d: %+v", len(prompt.Buttons), prompt.Buttons)
	}
	if prompt.Buttons[0].Label != "Mercado Pago" || prompt.Buttons[0].Data != askOptionPrefix+"0" {
		t.Errorf("el primer botón no es la primera opción: %+v", prompt.Buttons[0])
	}
	// El callback lleva el ÍNDICE, no el texto: callback_data tiene 64 bytes
	// y un nombre de categoría largo los desborda.
	if prompt.Buttons[1].Data != askOptionPrefix+"1" {
		t.Errorf("el callback tiene que ser el índice: %+v", prompt.Buttons[1])
	}
	if prompt.Buttons[2].Data != optionCancel {
		t.Errorf("falta el Cancelar: %+v", prompt.Buttons[2])
	}
}

func TestAskUser_CancelMarksCancelled(t *testing.T) {
	engine := newAskUserTestEngine(t)
	seed := askSeed(t, 3, pendingaction.OpenQuestion{Key: "row0.account", Prompt: "¿De qué cuenta salió?"})
	if _, err := engine.StartWithData(1, askUserFlowName, seed); err != nil {
		t.Fatal(err)
	}
	res, _, err := engine.Handle(1, conversation.Input{CallbackData: optionCancel})
	if err != nil || !res.Finished {
		t.Fatalf("cancel: res=%+v err=%v", res, err)
	}
	if !flag(res.Data, keyCancelled) {
		t.Errorf("cancelar tiene que dejar la marca: %+v", res.Data)
	}
	if flag(res.Data, keyAskDiscarded) {
		t.Errorf("cancelar NO es lo mismo que quedarse sin presupuesto: %+v", res.Data)
	}
}

// TestAskUser_SkipsWhenNothingIsOpen: el seed sin preguntas abiertas no puede
// arrancar el flujo — StartWithData falla a propósito cuando no hay nada que
// preguntar, y ese es el contrato que el drenaje usa para no abrir un flow vacío.
func TestAskUser_SkipsWhenNothingIsOpen(t *testing.T) {
	engine := newAskUserTestEngine(t)
	seed := askSeed(t, 3, pendingaction.OpenQuestion{
		Key: "row0.account", Prompt: "¿De qué cuenta salió?", Answer: "Brubank",
	})
	if _, err := engine.StartWithData(1, askUserFlowName, seed); err == nil {
		t.Fatal("sin preguntas abiertas StartWithData tiene que fallar, no mostrar un prompt vacío")
	}
}

// TestAskUser_BudgetExhaustedDiscardsWhole: pasado el techo, la acción se tira
// ENTERA y queda la marca. Task 4 es quien le avisa al usuario qué se cayó.
func TestAskUser_BudgetExhaustedDiscardsWhole(t *testing.T) {
	engine := newAskUserTestEngine(t)
	// Presupuesto 1 con 2 preguntas: la primera respuesta lo consume y la
	// segunda ya no se llega a preguntar.
	seed := askSeed(t, 1,
		pendingaction.OpenQuestion{Key: "row0.account", Prompt: "¿De qué cuenta salió?"},
		pendingaction.OpenQuestion{Key: "row0.category", Prompt: "¿En qué categoría lo pongo?"},
	)
	if _, err := engine.StartWithData(1, askUserFlowName, seed); err != nil {
		t.Fatal(err)
	}
	res, _, err := engine.Handle(1, conversation.Input{Text: "Brubank"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Finished {
		t.Fatal("agotado el presupuesto el flujo termina, no sigue preguntando")
	}
	if !flag(res.Data, keyAskDiscarded) {
		t.Errorf("falta la marca de descarte: %+v", res.Data)
	}
	if flag(res.Data, keyCancelled) {
		t.Errorf("quedarse sin presupuesto NO es que el usuario cancele: %+v", res.Data)
	}
	// Lo que sí se contestó tiene que seguir ahí: Task 4 nombra lo que se cae.
	got := decodeOpenQuestions(res.Data)
	if len(got) != 2 || got[0].Answer != "Brubank" || got[1].Answer != "" {
		t.Errorf("el estado de las preguntas no sobrevivió al descarte: %+v", got)
	}
}
