package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"lopiibot.com/internal/constants"
)

// ClassifyRow es una fila ya extraída, lista para clasificar. Sin merchant: la
// columna murió en el fold.
type ClassifyRow struct {
	Description string
	Type        string
	AccountName string
}

// Pair es un par (categoría, subcategoría) resuelto.
type Pair struct {
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
}

const classifierSystemPromptTemplate = `Sos el clasificador de un bot de finanzas personales argentino.
Recibís el mensaje original del usuario y los movimientos que ya se extrajeron de él.
Tu ÚNICO trabajo es elegir, para CADA movimiento y en el mismo orden, un par (categoría | subcategoría) de la lista de abajo.

REGLAS:
1. El par tiene que estar EXACTAMENTE como figura en la lista. No inventes, no traduzcas, no cambies mayúsculas.
2. Si no estás seguro, devolvé "%s" como categoría. Es una respuesta válida y esperada: el usuario elige después. Es MUCHO mejor que adivinar.
3. Usá el mensaje original, no sólo la descripción: "el asado del domingo con los chicos" dice más que "asado".
4. Devolvé exactamente un par por movimiento, en orden.

%s`

var classifierTool = toolSchema{
	Name:        "classify",
	Description: "Devuelve el par categoría/subcategoría de cada movimiento, en orden.",
	Parameters: json.RawMessage(`{
		"type": "object",
		"required": ["pairs"],
		"properties": {
			"pairs": {
				"type": "array",
				"items": {
					"type": "object",
					"required": ["category", "subcategory"],
					"properties": {
						"category": {"type": "string"},
						"subcategory": {"type": "string"}
					}
				}
			}
		}
	}`),
}

// ClassifyCategories clasifica TODOS los movimientos de un mensaje en UNA
// llamada.
//
// Una por mensaje y no una por movimiento: comparten el mismo texto, así que
// separarlas multiplicaría el costo y le sacaría a cada una el contexto que las
// otras aportan.
//
// Nunca devuelve error al llamador por una falla del modelo: degrada a
// PENDING_REVIEW en todas las filas, que abre el picker que ya existe. Degradar
// a una pregunta es correcto; degradar a un dato inventado no.
func (o *Orchestrator) ClassifyCategories(ctx context.Context, message string, rows []ClassifyRow, taxonomy []TaxonomyEntry) []Pair {
	if len(rows) == 0 {
		return nil
	}
	if len(taxonomy) == 0 {
		// Sin taxonomía no hay contra qué clasificar; preguntar es lo único honesto.
		return pendingPairs(len(rows))
	}

	systemPrompt := fmt.Sprintf(classifierSystemPromptTemplate,
		constants.PendingReview, buildTaxonomyBlock(taxonomy))

	raw, err := o.client.chatCompletion(ctx, callTypeClassifier, o.classifierModel,
		systemPrompt, renderClassifyInput(message, rows), classifierTool)
	if err != nil {
		return pendingPairs(len(rows))
	}

	var out struct {
		Pairs []Pair `json:"pairs"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return pendingPairs(len(rows))
	}

	// Un par por fila, en orden. Si el modelo devolvió de menos, el resto va a
	// preguntar; si devolvió de más, sobra y se descarta.
	pairs := make([]Pair, len(rows))
	for i := range pairs {
		if i < len(out.Pairs) && out.Pairs[i].Category != "" {
			pairs[i] = out.Pairs[i]
			continue
		}
		pairs[i] = Pair{Category: constants.PendingReview}
	}
	return pairs
}

func pendingPairs(n int) []Pair {
	out := make([]Pair, n)
	for i := range out {
		out[i] = Pair{Category: constants.PendingReview}
	}
	return out
}

// renderClassifyInput arma el user message: el texto original primero, porque
// es el que trae el contexto, y las filas numeradas para fijar el orden.
func renderClassifyInput(message string, rows []ClassifyRow) string {
	var b strings.Builder
	b.WriteString("MENSAJE DEL USUARIO:\n")
	b.WriteString(message)
	b.WriteString("\n\nMOVIMIENTOS A CLASIFICAR:\n")
	for i, r := range rows {
		fmt.Fprintf(&b, "%d. %s", i+1, r.Description)
		if r.Type != "" {
			fmt.Fprintf(&b, " (%s)", r.Type)
		}
		if r.AccountName != "" {
			fmt.Fprintf(&b, " · %s", r.AccountName)
		}
		b.WriteString("\n")
	}
	return b.String()
}
