package messaging

import (
	"context"
	"strings"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
)

// activeUser: un usuario que viene cargando fuerte, con una sola cuenta en
// pesos. Sirve de base para los tests del menú.
func activeUser() (*controller, *nudgeStats) {
	days := []movement.DayCount{daysAgo(0, 20)}
	c := &controller{
		// dayCounts va también en el fake: sendQuestionMenu NO recibe el stats,
		// lo reconstruye desde el repo para que el menú refleje los datos de
		// este momento y no los de cuando salió el tip.
		movements: &fakeMovementRepoFull{countForUser: 40, dayCounts: days},
		accounts:  &fakeAccountRepoFull{byUserID: []account.Account{{Name: "Galicia", Currency: currency.ARS}}},
	}
	s := &nudgeStats{total: 40, days: days, sent: map[string]bool{}}
	return c, s
}

// El menú solo ofrece preguntas que hoy tienen datos: sin cuenta USD, la
// pregunta por los dólares no aparece.
func TestEligibleQuestions_SkipsWhatWouldAnswerEmpty(t *testing.T) {
	c, s := activeUser()

	for _, n := range c.eligibleQuestions(1, s) {
		if n.key == nudgeUsdHoldingsTip {
			t.Error("el menú no debería ofrecer los dólares sin cuenta USD")
		}
		if n.question == "" {
			t.Errorf("el menú solo lleva preguntas, entró %q", n.key)
		}
		if n.recurring {
			t.Error("el menú no debería ofrecerse a sí mismo")
		}
	}
}

// El menú espera a que no quede ninguna pregunta ELEGIBLE sin mandar: los tips
// específicos enseñan la frase y el menú no, así que van primero.
func TestMenuTipGate_WaitsWhileAnEligibleQuestionIsUnsent(t *testing.T) {
	c, s := activeUser()

	if len(c.eligibleQuestions(1, s)) == 0 {
		t.Fatal("el fixture debería tener preguntas elegibles")
	}
	if gateFor(t, nudgeMenuTip)(c, 1, s) {
		t.Error("el menú no debería salir con preguntas elegibles sin mandar")
	}

	for _, n := range c.eligibleQuestions(1, s) {
		s.sent[n.key] = true
	}
	if !gateFor(t, nudgeMenuTip)(c, 1, s) {
		t.Error("el menú debería salir una vez ofrecidas todas las elegibles")
	}
}

// EL BUG QUE ESTE GATE EVITA: un usuario de una sola cuenta nunca cumple el
// gate de query_balance_tip. Con el gate ingenuo ("todos los tips mandados")
// su contador de pendientes no llegaba a cero jamás y el menú no salía NUNCA
// — justo para quien más lo necesita.
func TestMenuTipGate_ReachableBySingleAccountUser(t *testing.T) {
	c, s := activeUser()
	for _, n := range c.eligibleQuestions(1, s) {
		s.sent[n.key] = true
	}

	if s.sent[nudgeBalanceTip] {
		t.Fatal("con una sola cuenta, query_balance_tip no debería ser elegible")
	}
	if !gateFor(t, nudgeMenuTip)(c, 1, s) {
		t.Error("el menú tiene que ser alcanzable aunque haya tips que este usuario nunca reciba")
	}
}

// El menú sale en orden de declaración —que es orden de valor— y recorta
// DESPUÉS. Antes barajaba primero, así que podía tirar las mejores preguntas y
// además movía los botones de lugar en cada tap.
func TestSendQuestionMenu_KeepsTheBestInDeclaredOrder(t *testing.T) {
	c, _ := activeUser()
	c.orchestrator = &stubQueryOrchestrator{}
	c.nudges = &stubNudgeRepo{}
	c.chatHistory = stubChatHistory{}

	want := c.eligibleQuestions(1, c.buildNudgeStats(1))
	if len(want) < 2 {
		t.Fatalf("el fixture necesita al menos 2 preguntas elegibles, hay %d", len(want))
	}
	if len(want) > menuMaxOptions {
		want = want[:menuMaxOptions]
	}

	b, rt := newNudgeTestBot(t)
	c.sendQuestionMenu(context.Background(), b, 1, 1)
	if len(rt.markups) != 1 {
		t.Fatalf("expected 1 message, got %d", len(rt.markups))
	}

	markup := rt.markups[0]
	at := -1
	for _, n := range want {
		i := strings.Index(markup, n.question)
		if i < 0 {
			t.Fatalf("falta la pregunta %q en el menú: %s", n.question, markup)
		}
		if i < at {
			t.Errorf("el menú no respeta el orden de declaración: %q salió antes de lo que le toca", n.question)
		}
		at = i
	}
}

// El tap de "Preguntame" manda el menú, no una consulta.
func TestHandleNudgeQuery_MenuCallbackSendsTheMenu(t *testing.T) {
	orch := &stubQueryOrchestrator{}
	c, _ := activeUser()
	c.orchestrator = orch
	c.nudges = &stubNudgeRepo{}
	c.chatHistory = stubChatHistory{}

	b, rt := newNudgeTestBot(t)
	if !c.handleNudgeQuery(context.Background(), b, 1, 1, nudgeMenuData) {
		t.Fatal("el callback del menú tiene que estar manejado")
	}
	if orch.asked != "" {
		t.Errorf("el menú no corre ninguna consulta, pero preguntó %q", orch.asked)
	}
	if len(rt.texts) != 1 {
		t.Fatalf("expected 1 message, got %d: %+v", len(rt.texts), rt.texts)
	}
	if rt.texts[0] != msgMenuHeader {
		t.Errorf("expected the menu header, got %q", rt.texts[0])
	}
	if rt.markups[0] == "" {
		t.Error("el menú tiene que llevar botones")
	}
}
