package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"lopiibot.com/internal/constants"
)

type ClassifyRow struct {
	Description string
	Type        string
	AccountName string
}

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

func (o *Orchestrator) ClassifyCategories(ctx context.Context, message string, rows []ClassifyRow, taxonomy []TaxonomyEntry) []Pair {
	if len(rows) == 0 {
		return nil
	}
	if len(taxonomy) == 0 {
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
