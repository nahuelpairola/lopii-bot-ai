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

func TestNormalizeSubcategory_EmptyIsSafe(t *testing.T) {
	d := MovementDraft{}
	d.normalizeSubcategory()
	if d.Subcategory != "" {
		t.Errorf("Subcategory = %q, want vacía", d.Subcategory)
	}
}
