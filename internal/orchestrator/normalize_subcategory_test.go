package orchestrator

import "testing"

func TestNormalizeSubcategory_StripsEchoedCategoryPrefix(t *testing.T) {
	d := MovementDraft{Category: "Tecnología", Subcategory: "Tecnología | Electrodomésticos"}
	d.normalizeSubcategory()
	if d.Subcategory != "Electrodomésticos" {
		t.Errorf("Subcategory = %q, want %q", d.Subcategory, "Electrodomésticos")
	}
	if d.Category != "Tecnología" {
		t.Errorf("no debería tocar la categoría, quedó %q", d.Category)
	}
}

func TestNormalizeSubcategory_LeavesCorrectValueAlone(t *testing.T) {
	d := MovementDraft{Category: "Tecnología", Subcategory: "Electrodomésticos"}
	d.normalizeSubcategory()
	if d.Subcategory != "Electrodomésticos" {
		t.Errorf("Subcategory = %q, want intacta", d.Subcategory)
	}
}

func TestNormalizeSubcategory_UnrelatedPipeIsKept(t *testing.T) {
	d := MovementDraft{Category: "Tecnología", Subcategory: "Otra cosa | con pipe"}
	d.normalizeSubcategory()
	if d.Subcategory != "Otra cosa | con pipe" {
		t.Errorf("Subcategory = %q, want intacta (el pipe no es un eco)", d.Subcategory)
	}
}

func TestNormalizeSubcategory_SystemTransfer(t *testing.T) {
	d := MovementDraft{Category: "Sistema", Subcategory: "Sistema | Transferencia"}
	d.normalizeSubcategory()
	if d.Subcategory != "Transferencia" {
		t.Errorf("Subcategory = %q, want %q", d.Subcategory, "Transferencia")
	}
}

func TestNormalizeSubcategory_StripsPairFromCategory(t *testing.T) {
	d := MovementDraft{Category: "Ingresos | Freelance / honorarios", Subcategory: "Freelance / honorarios"}
	d.normalizeSubcategory()
	if d.Category != "Ingresos" {
		t.Errorf("Category = %q, want %q", d.Category, "Ingresos")
	}
	if d.Subcategory != "Freelance / honorarios" {
		t.Errorf("Subcategory = %q, want intacta", d.Subcategory)
	}
}

func TestNormalizeSubcategory_SplitsThePairWhenItIsInBothFields(t *testing.T) {
	for _, tt := range []struct{ in, wantCat, wantSub string }{
		{"Vivienda | Luz", "Vivienda", "Luz"},
		{"Ingresos | Freelance / honorarios", "Ingresos", "Freelance / honorarios"},
	} {
		d := MovementDraft{Category: tt.in, Subcategory: tt.in}
		d.normalizeSubcategory()
		if d.Category != tt.wantCat || d.Subcategory != tt.wantSub {
			t.Errorf("%q → (%q, %q), want (%q, %q)", tt.in, d.Category, d.Subcategory, tt.wantCat, tt.wantSub)
		}
	}
}

func TestNormalizeSubcategory_EqualFieldsWithoutPipeAreKept(t *testing.T) {
	d := MovementDraft{Category: "Otros", Subcategory: "Otros"}
	d.normalizeSubcategory()
	if d.Category != "Otros" || d.Subcategory != "Otros" {
		t.Errorf("quedó (%q, %q), want intactos", d.Category, d.Subcategory)
	}
}

func TestNormalizeSubcategory_UnrelatedPipeInCategoryIsKept(t *testing.T) {
	d := MovementDraft{Category: "Deudas | préstamos", Subcategory: "Tarjeta"}
	d.normalizeSubcategory()
	if d.Category != "Deudas | préstamos" {
		t.Errorf("Category = %q, want intacta", d.Category)
	}
}

func TestNormalizeSubcategory_EmptySubcategoryLeavesCategoryAlone(t *testing.T) {
	d := MovementDraft{Category: "Ingresos | Sueldo"}
	d.normalizeSubcategory()
	if d.Category != "Ingresos | Sueldo" {
		t.Errorf("Category = %q, want intacta", d.Category)
	}
}

func TestNormalizeSubcategory_EmptyIsSafe(t *testing.T) {
	d := MovementDraft{}
	d.normalizeSubcategory()
	if d.Subcategory != "" {
		t.Errorf("Subcategory = %q, want vacía", d.Subcategory)
	}
}
