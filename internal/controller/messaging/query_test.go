package messaging

import (
	"strings"
	"testing"

	"lopiibot.com/internal/subcategory"
)

// TestExecListCategories_OmitsEmptyDescription es el gemelo de
// TestBuildTaxonomyBlock_OmitsEmptyDescription en el orchestrator. Los dos
// formatean la taxonomía con el mismo "cat | sub | desc", pero ESTE va en la
// respuesta que lee el usuario cuando pregunta qué categorías hay. Después de
// la migración de podado, 26 de las 65 globales tienen descripción vacía y sin
// la guarda salen con un separador colgante.
//
// Va por el camino FILTRADO (Category), que es el único que manda descripciones
// desde el recorte de tokens del 2026-08-10: la lista completa ya no las trae, así
// que ahí el separador colgante es imposible por construcción.
func TestExecListCategories_OmitsEmptyDescription(t *testing.T) {
	subs := &fakeSubcategoryRepoFull{all: []subcategory.Subcategory{
		{Category: "Alimentación", Subcategory: "Supermercado", Description: "Compra grande. NO incluye almacén.", Icon: "🍔"},
		{Category: "Alimentación", Subcategory: "Sueldo", Icon: "💵"},
	}}
	c := &controller{subcategories: subs}

	out, err := c.execListCategories(1, queryToolArgs{Category: "Alimentación"})
	if err != nil {
		t.Fatalf("execListCategories: %v", err)
	}

	if !strings.Contains(out, "Alimentación | Supermercado | Compra grande. NO incluye almacén.") {
		t.Errorf("un par con nota tiene que conservarla:\n%s", out)
	}
	if strings.Contains(out, "Sueldo | ") {
		t.Errorf("separador colgante en la respuesta al usuario:\n%s", out)
	}
	if !strings.Contains(out, "Alimentación | Sueldo") {
		t.Errorf("el par sin nota igual tiene que estar:\n%s", out)
	}
}
