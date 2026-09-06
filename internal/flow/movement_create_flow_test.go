package flow

import (
	"strings"
	"testing"

	"lopiibot.com/internal/account"
	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/currency"
	"lopiibot.com/internal/movement"
	"lopiibot.com/internal/subcategory"
)

type fakeGapAccountRepo struct{}

func (fakeGapAccountRepo) FindDefaultByCurrency(uint64, currency.Currency) (*account.Account, error) {
	return &account.Account{}, nil
}
func (fakeGapAccountRepo) HasDefaultForCurrency(uint64, currency.Currency) bool { return true }
func (fakeGapAccountRepo) FindByUserID(uint64) ([]account.Account, error)       { return nil, nil }

type fakeGapSubcategoryRepo struct {
	categories []string
	all        []subcategory.Subcategory
}

func (r fakeGapSubcategoryRepo) FindByCategoryAndSubcategory(uint64, string, string) (*subcategory.Subcategory, error) {
	return nil, subcategory.ErrSubcategoryNotFound
}
func (r fakeGapSubcategoryRepo) FindAllForUser(uint64) ([]subcategory.Subcategory, error) {
	return r.all, nil
}
func (r fakeGapSubcategoryRepo) DistinctCategoriesForUser(uint64) ([]string, error) {
	return r.categories, nil
}
func (r fakeGapSubcategoryRepo) IconForCategory(uint64, string) string { return "🩺" }

func saludTaxonomy() fakeGapSubcategoryRepo {
	return fakeGapSubcategoryRepo{
		categories: []string{"Salud", "Vivienda"},
		all: []subcategory.Subcategory{
			{Category: "Salud", Subcategory: "Odontología"},
			{Category: "Salud", Subcategory: "Psicología"},
			{Category: "Vivienda", Subcategory: "Alquiler"},
		},
	}
}

func startAtSubcategoryGap(t *testing.T) *conversation.Engine {
	t.Helper()
	engine := conversation.NewEngine(&fakeStateStore{}, func(string) string { return "algo" })
	engine.Register(NewMovementCreateFlow(saludTaxonomy(), fakeGapAccountRepo{}))

	seed := conversation.Data{
		conversation.UserIDKey: uint64(1),
		conversation.KeyMode:   ModeUpdate,
		conversation.KeyMovements: movement.EncodeMovementRows([]movement.MovementRow{
			{Type: "expense", Amount: "80990", Currency: "ARS", Date: "2026-09-06", Category: "Salud"},
		}),
		conversation.KeyPendingCategoryGaps: conversation.EncodeStringSlice([]string{"0"}),
	}
	prompt, err := engine.StartWithData(1, MovementCreateFlowName, seed)
	if err != nil {
		t.Fatalf("StartWithData: %v", err)
	}
	if !strings.Contains(prompt.Text, "subcategoría") {
		t.Fatalf("el flow no arrancó en el paso de subcategoría: %q", prompt.Text)
	}
	return engine
}

func TestMovementCreate_SubcategoryGap_BackReturnsToTheCategoryList(t *testing.T) {
	engine := startAtSubcategoryGap(t)

	result, found, err := engine.Handle(1, conversation.Input{CallbackData: OptionBack})
	if err != nil || !found {
		t.Fatalf("Handle: found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("⬅️ Atrás terminó el flow: la corrección se perdió")
	}
	if !strings.Contains(result.Prompt.Text, "categoría") || strings.Contains(result.Prompt.Text, "subcategoría") {
		t.Errorf("⬅️ Atrás no volvió al listado de categorías, rebotó a: %q", result.Prompt.Text)
	}
}

func TestMovementCreate_SubcategoryGap_OtraAsksForTheNameAndClosesTheGap(t *testing.T) {
	engine := startAtSubcategoryGap(t)

	result, found, err := engine.Handle(1, conversation.Input{CallbackData: OptionSubcategoryCreate})
	if err != nil || !found {
		t.Fatalf("Handle ➕ Otra: found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("➕ Otra terminó el flow sin pedir el nombre")
	}

	result, found, err = engine.Handle(1, conversation.Input{Text: "Alojamiento por tratamiento"})
	if err != nil || !found {
		t.Fatalf("Handle nombre: found=%v err=%v", found, err)
	}
	if !result.Finished {
		t.Fatalf("el flow no terminó con el gap cerrado: %q", result.Prompt.Text)
	}

	rows := movement.DecodeMovementRows(result.Data)
	if rows[0].Category != "Salud" || rows[0].Subcategory != "Alojamiento por tratamiento" {
		t.Errorf("la fila quedó en %q/%q, want Salud/Alojamiento por tratamiento", rows[0].Category, rows[0].Subcategory)
	}
	if got := conversation.DecodeStringSlice(result.Data, conversation.KeyPendingNewSubcats); len(got) != 1 || got[0] != "0" {
		t.Errorf("la subcategoría nueva no quedó marcada para crear: %v", got)
	}
	if len(conversation.DecodeStringSlice(result.Data, conversation.KeyPendingCategoryGaps)) != 0 {
		t.Error("el gap de categoría siguió abierto")
	}
}

func TestMovementCreate_NewSubcategoryName_RejectsNamesThatBlowTheCallbackLimit(t *testing.T) {
	engine := startAtSubcategoryGap(t)
	if _, _, err := engine.Handle(1, conversation.Input{CallbackData: OptionSubcategoryCreate}); err != nil {
		t.Fatalf("Handle ➕ Otra: %v", err)
	}

	long := strings.Repeat("á", MaxSubcategoryNameRunes+1)
	result, found, err := engine.Handle(1, conversation.Input{Text: long})
	if err != nil || !found {
		t.Fatalf("Handle nombre largo: found=%v err=%v", found, err)
	}
	if result.Finished {
		t.Fatal("aceptó un nombre que no entra en los 64 bytes de callback_data: el botón queda mudo")
	}
	if !strings.Contains(result.Prompt.Text, "largo") {
		t.Errorf("no explicó por qué lo rechaza: %q", result.Prompt.Text)
	}
}
