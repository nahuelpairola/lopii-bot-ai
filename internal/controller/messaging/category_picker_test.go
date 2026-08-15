package messaging

import (
	"errors"
	"testing"

	"lopiibot.com/internal/conversation"
	"lopiibot.com/internal/flow"
)

type fakeCategoryLister struct {
	cats []string
	err  error
}

func (f fakeCategoryLister) DistinctCategoriesForUser(uint64) ([]string, error) {
	return f.cats, f.err
}

func (f fakeCategoryLister) IconForCategory(_ uint64, category string) string {
	if category == "Alimentos" {
		return "🥑"
	}
	return "📂"
}

var errFake = errors.New("db down")

func TestCategoryOptions_OneOptionPerCategoryPlusExtras(t *testing.T) {
	subs := fakeCategoryLister{cats: []string{"Alimentos", "Hogar"}}
	opts := flow.CategoryOptions(subs, conversation.Data{}, "next_step", flow.CancelOption)

	if len(opts) != 3 {
		t.Fatalf("len(opts) = %d, want 3 (2 categorías + cancelar)", len(opts))
	}
	if opts[0].Label != "🥑 Alimentos" {
		t.Errorf("opts[0].Label = %q, want %q", opts[0].Label, "🥑 Alimentos")
	}
	if opts[0].Value != "Alimentos" {
		t.Errorf("opts[0].Value = %q, want %q", opts[0].Value, "Alimentos")
	}
	if opts[0].NextStep != "next_step" {
		t.Errorf("opts[0].NextStep = %q, want %q", opts[0].NextStep, "next_step")
	}
	if opts[1].Label != "📂 Hogar" {
		t.Errorf("opts[1].Label = %q, want %q", opts[1].Label, "📂 Hogar")
	}
	if opts[2].Value != flow.OptionCancel {
		t.Errorf("última opción = %q, want %q", opts[2].Value, flow.OptionCancel)
	}
}

func TestCategoryOptions_NoExtrasIsJustCategories(t *testing.T) {
	subs := fakeCategoryLister{cats: []string{"Alimentos"}}
	opts := flow.CategoryOptions(subs, conversation.Data{}, "next_step")
	if len(opts) != 1 {
		t.Fatalf("len(opts) = %d, want 1", len(opts))
	}
}

// Sin esto, un error de DB dejaría al usuario con cero botones y sin salida.
func TestCategoryOptions_ListerErrorStillReturnsExtras(t *testing.T) {
	subs := fakeCategoryLister{err: errFake}
	opts := flow.CategoryOptions(subs, conversation.Data{}, "next_step", flow.CancelOption)
	if len(opts) != 1 || opts[0].Value != flow.OptionCancel {
		t.Errorf("con error del lister, opts = %+v, want solo cancelar", opts)
	}
}

func TestBackOptionTo_TargetsGivenStep(t *testing.T) {
	opt := flow.BackOptionTo("algun_step")
	if opt.Value != flow.OptionBack {
		t.Errorf("Value = %q, want %q", opt.Value, flow.OptionBack)
	}
	if opt.NextStep != "algun_step" {
		t.Errorf("NextStep = %q, want %q", opt.NextStep, "algun_step")
	}
	if opt.Finish {
		t.Error("Atrás no debería terminar el flujo")
	}
}
