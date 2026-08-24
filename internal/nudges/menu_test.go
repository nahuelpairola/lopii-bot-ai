package nudges

import (
	"context"
	"strings"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/messenger"
	"lopiibot.com/internal/movement"
)

// activeUser: un usuario que viene cargando fuerte, con una sola cuenta en
// pesos. Sirve de base para los tests del menú.
func activeUser() (*testServices, *nudgeStats) {
	days := []movement.DayCount{daysAgo(0, 20)}
	svc := &testServices{
		// dayCounts va también en el fake: sendQuestionMenu NO recibe el stats,
		// lo reconstruye desde el repo para que el menú refleje los datos de
		// este momento y no los de cuando salió el tip.
		counts:     40,
		dayCounts:  days,
		accountsBy: []account.Account{{Name: "Galicia", Currency: currency.ARS}},
	}
	s := &nudgeStats{total: 40, days: days, sent: map[string]bool{}}
	return svc, s
}

// El menú solo ofrece preguntas que hoy tienen datos: sin cuenta USD, la
// pregunta por los dólares no aparece.
func TestEligibleQuestions_SkipsWhatWouldAnswerEmpty(t *testing.T) {
	svc, s := activeUser()

	for _, n := range eligibleQuestions(svc, 1, s) {
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
	svc, s := activeUser()

	if len(eligibleQuestions(svc, 1, s)) == 0 {
		t.Fatal("el fixture debería tener preguntas elegibles")
	}
	if gateFor(t, nudgeMenuTip)(svc, 1, s) {
		t.Error("el menú no debería salir con preguntas elegibles sin mandar")
	}

	for _, n := range eligibleQuestions(svc, 1, s) {
		s.sent[n.key] = true
	}
	if !gateFor(t, nudgeMenuTip)(svc, 1, s) {
		t.Error("el menú debería salir una vez ofrecidas todas las elegibles")
	}
}

// EL BUG QUE ESTE GATE EVITA: un usuario de una sola cuenta nunca cumple el
// gate de query_balance_tip. Con el gate ingenuo ("todos los tips mandados")
// su contador de pendientes no llegaba a cero jamás y el menú no salía NUNCA
// — justo para quien más lo necesita.
func TestMenuTipGate_ReachableBySingleAccountUser(t *testing.T) {
	svc, s := activeUser()
	for _, n := range eligibleQuestions(svc, 1, s) {
		s.sent[n.key] = true
	}

	if s.sent[nudgeBalanceTip] {
		t.Fatal("con una sola cuenta, query_balance_tip no debería ser elegible")
	}
	if !gateFor(t, nudgeMenuTip)(svc, 1, s) {
		t.Error("el menú tiene que ser alcanzable aunque haya tips que este usuario nunca reciba")
	}
}

// El menú sale en orden de declaración —que es orden de valor— y recorta
// DESPUÉS. Antes barajaba primero, así que podía tirar las mejores preguntas y
// además movía los botones de lugar en cada tap.
func TestSendQuestionMenu_KeepsTheBestInDeclaredOrder(t *testing.T) {
	svc, _ := activeUser()

	want := eligibleQuestions(svc, 1, buildNudgeStats(svc, 1))
	if len(want) < 2 {
		t.Fatalf("el fixture necesita al menos 2 preguntas elegibles, hay %d", len(want))
	}
	if len(want) > menuMaxOptions {
		want = want[:menuMaxOptions]
	}

	sendQuestionMenu(context.Background(), svc, &messenger.FakeChat{}, 1)
	if len(svc.markups) != 1 {
		t.Fatalf("expected 1 message, got %d", len(svc.markups))
	}

	markup := svc.markups[0]
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
	svc, _ := activeUser()

	if !HandleCallback(context.Background(), svc, &messenger.FakeChat{}, 1, nudgeMenuData) {
		t.Fatal("el callback del menú tiene que estar manejado")
	}
	if svc.asked != "" {
		t.Errorf("el menú no corre ninguna consulta, pero preguntó %q", svc.asked)
	}
	if len(svc.texts) != 1 {
		t.Fatalf("expected 1 message, got %d: %+v", len(svc.texts), svc.texts)
	}
	if svc.texts[0] != msgMenuHeader {
		t.Errorf("expected the menu header, got %q", svc.texts[0])
	}
	if svc.markups[0] == "" {
		t.Error("el menú tiene que llevar botones")
	}
}
