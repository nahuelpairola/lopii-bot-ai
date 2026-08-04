package orchestrator

import "testing"

// El modelo devuelve de a ratos "Categoría | Subcategoría" en el campo
// subcategoría, imitando cómo se le serializa la taxonomía. Sin corregirlo, el
// par no matchea ninguna fila y al usuario se le pregunta la categoría que ya
// había dicho.
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

// Solo se saca el prefijo si coincide con la categoría del MISMO draft. Un pipe
// que no sea un eco de la categoría es parte legítima del nombre.
func TestNormalizeSubcategory_UnrelatedPipeIsKept(t *testing.T) {
	d := MovementDraft{Category: "Tecnología", Subcategory: "Otra cosa | con pipe"}
	d.normalizeSubcategory()
	if d.Subcategory != "Otra cosa | con pipe" {
		t.Errorf("Subcategory = %q, want intacta (el pipe no es un eco)", d.Subcategory)
	}
}

// Caso real del prompt: subcategoría "Sistema | Transferencia".
func TestNormalizeSubcategory_SystemTransfer(t *testing.T) {
	d := MovementDraft{Category: "Sistema", Subcategory: "Sistema | Transferencia"}
	d.normalizeSubcategory()
	if d.Subcategory != "Transferencia" {
		t.Errorf("Subcategory = %q, want %q", d.Subcategory, "Transferencia")
	}
}

// El error espejo, visto en producción el 2026-08-04: el par entero en el campo
// CATEGORÍA. "Ingresos | Freelance / honorarios" no matchea ninguna fila, así
// que el ingreso caía al gap-fill y el usuario terminaba eligiendo a mano una
// categoría equivocada.
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

// Tercera variante, vista en producción el 2026-08-04 (traza e8dc828c): el par
// entero en LOS DOS campos. Las dos filas del mensaje vinieron así y las dos
// abrieron gap.
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

// Dos campos iguales SIN pipe son un par legítimo raro, no un eco: no se tocan.
func TestNormalizeSubcategory_EqualFieldsWithoutPipeAreKept(t *testing.T) {
	d := MovementDraft{Category: "Otros", Subcategory: "Otros"}
	d.normalizeSubcategory()
	if d.Category != "Otros" || d.Subcategory != "Otros" {
		t.Errorf("quedó (%q, %q), want intactos", d.Category, d.Subcategory)
	}
}

// Una categoría con pipe que NO termina en la subcategoría del draft no es un
// eco: se deja como está.
func TestNormalizeSubcategory_UnrelatedPipeInCategoryIsKept(t *testing.T) {
	d := MovementDraft{Category: "Deudas | préstamos", Subcategory: "Tarjeta"}
	d.normalizeSubcategory()
	if d.Category != "Deudas | préstamos" {
		t.Errorf("Category = %q, want intacta", d.Category)
	}
}

// Con la subcategoría vacía no hay con qué comparar: la categoría no se toca.
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
