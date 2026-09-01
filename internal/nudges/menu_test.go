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

func activeUser() (*testServices, *nudgeStats) {
	days := []movement.DayCount{daysAgo(0, 20)}
	svc := &testServices{
		counts:     40,
		dayCounts:  days,
		accountsBy: []account.Account{{Name: "Galicia", Currency: currency.ARS}},
	}
	s := &nudgeStats{total: 40, days: days, sent: map[string]bool{}}
	return svc, s
}

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
