package messaging

import (
	"strings"
	"testing"
	"time"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/subcategory"
)

type fakeSubcatSetupRepo struct {
	categories []string
	existing   *subcategory.Subcategory
}

func (r *fakeSubcatSetupRepo) FindByCategoryAndSubcategory(userID uint64, category, sub string) (*subcategory.Subcategory, error) {
	if r.existing != nil && r.existing.Category == category && r.existing.Subcategory == sub {
		return r.existing, nil
	}
	return nil, subcategory.ErrSubcategoryNotFound
}
func (r *fakeSubcatSetupRepo) FindAllForUser(userID uint64) ([]subcategory.Subcategory, error) { return nil, nil }
func (r *fakeSubcatSetupRepo) DistinctCategoriesForUser(userID uint64) ([]string, error) {
	return r.categories, nil
}
func (r *fakeSubcatSetupRepo) IconForCategory(userID uint64, category string) string { return "📂" }
func (r *fakeSubcatSetupRepo) Insert(s *subcategory.Subcategory) error               { return nil }
func (r *fakeSubcatSetupRepo) Reload() error                                        { return nil }

// fakeConvStore is a minimal conversation.Engine-compatible store — Go
// interface satisfaction is structural, so this struct (defined in the
// messaging package) satisfies conversation's unexported stateStore
// interface purely by matching its method set, the same way
// internal/conversation/engine_test.go's own fakeStore does from inside
// that package.
type fakeConvStore struct {
	flowName, stepName string
	data               conversation.Data
	updatedAt          time.Time
	found              bool
}

func (s *fakeConvStore) Get(userID uint64) (string, string, conversation.Data, time.Time, bool, error) {
	return s.flowName, s.stepName, s.data, s.updatedAt, s.found, nil
}
func (s *fakeConvStore) Set(userID uint64, flowName, stepName string, data conversation.Data) error {
	s.flowName, s.stepName, s.data, s.found = flowName, stepName, data, true
	s.updatedAt = time.Now()
	return nil
}
func (s *fakeConvStore) Clear(userID uint64) error {
	s.found = false
	return nil
}

func newSubcategorySetupTestEngine(repo subcategoryRepository) *conversation.Engine {
	engine := conversation.NewEngine(&fakeConvStore{}, func(string) string { return "algo" })
	engine.Register(NewSubcategorySetupFlow(repo))
	return engine
}

func TestSubcategorySetup_ChooseMode_NewCategory_AdvancesToNameStep(t *testing.T) {
	engine := newSubcategorySetupTestEngine(&fakeSubcatSetupRepo{})
	if _, err := engine.Start(1, subcategorySetupFlowName); err != nil {
		t.Fatalf("Start: %v", err)
	}
	result, found, err := engine.Handle(1, conversation.Input{CallbackData: optionNewCategory})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if result.Prompt.Text != subcategory.MsgAskNewCategoryName() {
		t.Errorf("prompt = %q, want the new-category-name prompt", result.Prompt.Text)
	}
}

func TestSubcategorySetup_NewCategoryName_RejectsReservedNames(t *testing.T) {
	engine := newSubcategorySetupTestEngine(&fakeSubcatSetupRepo{})
	if _, err := engine.Start(1, subcategorySetupFlowName); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: optionNewCategory}); err != nil {
		t.Fatalf("Handle (choose mode): %v", err)
	}

	result, found, err := engine.Handle(1, conversation.Input{Text: "Sistema"})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("expected a Retry (reserved name), not completion")
	}
	if !strings.Contains(result.Prompt.Text, subcategory.MsgInvalidCategoryName) {
		t.Errorf("prompt = %q, want it to contain the invalid-name error", result.Prompt.Text)
	}
}

func TestSubcategorySetup_SeededStepsOfferConfirmButton(t *testing.T) {
	engine := newSubcategorySetupTestEngine(&fakeSubcatSetupRepo{})
	seed := conversation.Data{
		"category": "Regalos", "category_is_new": "true", "category_icon": "🎁",
		"subcategory": "Regalos", "subcategory_description": "Regalos a terceros.",
	}
	if _, err := engine.StartWithData(1, subcategorySetupFlowName, seed); err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	res, _, err := engine.Handle(1, conversation.Input{CallbackData: optionNewCategory})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, btn := range res.Prompt.Buttons {
		if strings.Contains(btn.Label, "Usar Regalos") {
			found = true
		}
	}
	if !found {
		t.Errorf("seeded name step missing confirm button; buttons = %+v", res.Prompt.Buttons)
	}
}

func TestSubcategorySetup_ExistingCategory_SkipsIconStep_LandsOnSubcategoryName(t *testing.T) {
	engine := newSubcategorySetupTestEngine(&fakeSubcatSetupRepo{categories: []string{"Mascotas"}})
	if _, err := engine.Start(1, subcategorySetupFlowName); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: "existing"}); err != nil {
		t.Fatalf("Handle (choose existing): %v", err)
	}
	result, found, err := engine.Handle(1, conversation.Input{CallbackData: "Mascotas"})
	if err != nil || !found {
		t.Fatalf("Handle (pick Mascotas): %v", err)
	}
	if result.Prompt.Text != subcategory.MsgAskSubcategoryName("Mascotas") {
		t.Errorf("prompt = %q, want to land directly on the subcategory-name step (icon step skipped)", result.Prompt.Text)
	}
}

func TestSubcategorySetup_SubcategoryName_RejectsDuplicate(t *testing.T) {
	existing := &subcategory.Subcategory{Category: "Mascotas", Subcategory: "Veterinario"}
	engine := newSubcategorySetupTestEngine(&fakeSubcatSetupRepo{categories: []string{"Mascotas"}, existing: existing})
	if _, err := engine.Start(1, subcategorySetupFlowName); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: "existing"}); err != nil {
		t.Fatalf("Handle (choose existing): %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: "Mascotas"}); err != nil {
		t.Fatalf("Handle (pick Mascotas): %v", err)
	}

	result, found, err := engine.Handle(1, conversation.Input{Text: "Veterinario"})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("expected a Retry (duplicate), not completion")
	}
	if !strings.Contains(result.Prompt.Text, subcategory.MsgSubcategoryAlreadyExists("Mascotas", "Veterinario")) {
		t.Errorf("prompt = %q, want it to contain the duplicate error", result.Prompt.Text)
	}
}

func TestSubcategorySetup_SubcategoryName_ThenDescription_AdvancesToConfirm(t *testing.T) {
	engine := newSubcategorySetupTestEngine(&fakeSubcatSetupRepo{})
	if _, err := engine.Start(1, subcategorySetupFlowName); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: optionNewCategory}); err != nil {
		t.Fatalf("Handle (choose mode): %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{Text: "Mascotas"}); err != nil {
		t.Fatalf("Handle (category name): %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{Text: "🐶"}); err != nil {
		t.Fatalf("Handle (category icon): %v", err)
	}
	result, found, err := engine.Handle(1, conversation.Input{Text: "Veterinario"})
	if err != nil || !found {
		t.Fatalf("Handle (subcategory name): found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("expected the description step, not completion")
	}

	result, found, err = engine.Handle(1, conversation.Input{Text: "Consultas y controles de mascotas"})
	if err != nil || !found {
		t.Fatalf("Handle (description): found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("expected the confirm step, not completion")
	}
	if !strings.Contains(result.Prompt.Text, "Mascotas") || !strings.Contains(result.Prompt.Text, "Veterinario") {
		t.Errorf("prompt = %q, want the confirm step showing category › subcategory", result.Prompt.Text)
	}
}

func TestSubcategorySetup_Description_RejectsEmpty(t *testing.T) {
	engine := newSubcategorySetupTestEngine(&fakeSubcatSetupRepo{})
	if _, err := engine.Start(1, subcategorySetupFlowName); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: optionNewCategory}); err != nil {
		t.Fatalf("Handle (choose mode): %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{Text: "Mascotas"}); err != nil {
		t.Fatalf("Handle (category name): %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{Text: "🐶"}); err != nil {
		t.Fatalf("Handle (category icon): %v", err)
	}
	if _, _, err := engine.Handle(1, conversation.Input{Text: "Veterinario"}); err != nil {
		t.Fatalf("Handle (subcategory name): %v", err)
	}

	result, found, err := engine.Handle(1, conversation.Input{Text: "   "})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("expected a Retry (empty description), not completion")
	}
}
