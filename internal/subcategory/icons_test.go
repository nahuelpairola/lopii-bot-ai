package subcategory

import "testing"

func TestIconFor_KnownCategory(t *testing.T) {
	if got := IconFor("Alimentación"); got != "🍔" {
		t.Errorf("IconFor(Alimentación) = %q, want 🍔", got)
	}
}

func TestIconFor_UnknownCategory_Fallback(t *testing.T) {
	if got := IconFor("Categoría inventada"); got != "📂" {
		t.Errorf("IconFor(unknown) = %q, want fallback 📂", got)
	}
}

func TestCategoryIcon_HasExactly16Entries(t *testing.T) {
	if len(CategoryIcon) != 16 {
		t.Errorf("len(CategoryIcon) = %d, want 16 (14 real categories + PENDING_REVIEW + Sistema)", len(CategoryIcon))
	}
}
