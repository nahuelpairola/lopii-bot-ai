package messaging

import (
	"strings"
	"testing"

	"lopiibot.com/internal/conversation"
)

func newCategoryFlowsTestEngine(t *testing.T) *conversation.Engine {
	t.Helper()
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(NewCategoryMatchOfferFlow())
	engine.Register(NewCategoryProposalConfirmFlow())
	return engine
}

func TestCategoryProposalConfirm_ConfirmFinishesWithSeed(t *testing.T) {
	engine := newCategoryFlowsTestEngine(t)
	seed := conversation.Data{
		"category": "Regalos", "subcategory": "Regalos",
		"subcategory_description": "Regalos a terceros.",
		"category_icon":           "🎁", "category_is_new": "true",
	}
	prompt, err := engine.StartWithData(1, categoryProposalConfirmFlowName, seed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.Text, "🎁 Regalos › Regalos") || !strings.Contains(prompt.Text, "Regalos a terceros.") {
		t.Errorf("proposal prompt missing pieces:\n%s", prompt.Text)
	}
	res, found, err := engine.Handle(1, conversation.Input{CallbackData: "confirm"})
	if err != nil || !found || !res.Finished {
		t.Fatalf("confirm: res=%+v found=%v err=%v", res, found, err)
	}
	if conversation.StringOrEmpty(res.Data["category"]) != "Regalos" || conversation.StringOrEmpty(res.Data["subcategory_description"]) != "Regalos a terceros." {
		t.Errorf("seed lost on finish: %+v", res.Data)
	}
	if conversation.StringOrEmpty(res.Data["edit_proposal"]) == "true" || conversation.StringOrEmpty(res.Data["cancelled"]) == "true" {
		t.Errorf("confirm must not set edit/cancel markers: %+v", res.Data)
	}
}

func TestCategoryProposalConfirm_EditSetsMarker(t *testing.T) {
	engine := newCategoryFlowsTestEngine(t)
	seed := conversation.Data{"category": "Regalos", "subcategory": "Regalos", "category_icon": "🎁"}
	if _, err := engine.StartWithData(1, categoryProposalConfirmFlowName, seed); err != nil {
		t.Fatal(err)
	}
	res, _, err := engine.Handle(1, conversation.Input{CallbackData: optionEditProposal})
	if err != nil || !res.Finished {
		t.Fatalf("edit: res=%+v err=%v", res, err)
	}
	if conversation.StringOrEmpty(res.Data["edit_proposal"]) != "true" {
		t.Errorf("edit_proposal marker not set: %+v", res.Data)
	}
}

func TestCategoryMatchOffer_UseExistingAndCreateNew(t *testing.T) {
	seed := conversation.Data{
		"category": "Otros", "subcategory": "Regalos / donaciones",
		"subcategory_description": "Regalos a terceros y donaciones.", "category_icon": "🗂️",
	}

	engine := newCategoryFlowsTestEngine(t)
	prompt, err := engine.StartWithData(1, categoryMatchOfferFlowName, seed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt.Text, "Ya tenés") || !strings.Contains(prompt.Text, "Otros › Regalos / donaciones") {
		t.Errorf("match prompt missing pieces:\n%s", prompt.Text)
	}
	res, _, err := engine.Handle(1, conversation.Input{CallbackData: optionUseExisting})
	if err != nil || !res.Finished {
		t.Fatalf("use_existing: res=%+v err=%v", res, err)
	}
	if conversation.StringOrEmpty(res.Data["match_choice"]) != optionUseExisting {
		t.Errorf("match_choice = %q, want %q", res.Data["match_choice"], optionUseExisting)
	}

	engine2 := newCategoryFlowsTestEngine(t)
	if _, err := engine2.StartWithData(1, categoryMatchOfferFlowName, seed); err != nil {
		t.Fatal(err)
	}
	res2, _, err := engine2.Handle(1, conversation.Input{CallbackData: optionCreateNew})
	if err != nil || !res2.Finished {
		t.Fatalf("create_new: res=%+v err=%v", res2, err)
	}
	if conversation.StringOrEmpty(res2.Data["match_choice"]) != optionCreateNew {
		t.Errorf("match_choice = %q, want %q", res2.Data["match_choice"], optionCreateNew)
	}
}

func TestCategoryFlows_CancelSetsCancelled(t *testing.T) {
	for _, flow := range []string{categoryMatchOfferFlowName, categoryProposalConfirmFlowName} {
		engine := newCategoryFlowsTestEngine(t)
		seed := conversation.Data{"category": "X", "subcategory": "Y"}
		if _, err := engine.StartWithData(1, flow, seed); err != nil {
			t.Fatalf("%s Start: %v", flow, err)
		}
		res, _, err := engine.Handle(1, conversation.Input{CallbackData: optionCancel})
		if err != nil || !res.Finished {
			t.Fatalf("%s cancel: res=%+v err=%v", flow, res, err)
		}
		if conversation.StringOrEmpty(res.Data["cancelled"]) != "true" {
			t.Errorf("%s: cancelled marker not set: %+v", flow, res.Data)
		}
	}
}
