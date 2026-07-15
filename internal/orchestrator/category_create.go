package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

const categoryCreateSystemPromptTemplate = `Sos el administrador de la taxonomía de categorías de una app de finanzas personales para un individuo en Argentina.
El usuario pidió crear una categoría o subcategoría nueva. Decidí UNA de dos salidas:
1. match: si el pedido ya está cubierto semánticamente por una entrada de la TAXONOMÍA EXISTENTE, devolvé esa entrada copiando categoría y subcategoría EXACTAMENTE como figuran.
2. proposal: si no está cubierto, proponé la subcategoría nueva completa: en qué categoría va (una existente de la lista si encaja; si ninguna encaja, una categoría nueva), el nombre de la subcategoría, un emoji representativo (uno solo), y una descripción de UNA frase que diga cuándo aplica — es la guía que un clasificador automático usa para mandar gastos futuros ahí (ej: "Regalos a terceros y donaciones.").

REGLAS:
- Nombres en español argentino, cortos, primera letra en mayúscula.
- PROHIBIDO usar "Sistema" o "PENDING_REVIEW" como categoría o subcategoría.
- Devolvé exactamente uno de los dos campos: match O proposal, nunca ambos, nunca ninguno.

TAXONOMÍA EXISTENTE (categoría | subcategoría | descripción):
%s`

var categoryCreateTool = toolSchema{
	Name:        "resolve_category",
	Description: "Resuelve el pedido de categoría nueva: match contra la taxonomía existente o propuesta completa",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"match": {
				"type": ["object", "null"],
				"properties": {
					"category": {"type": "string"},
					"subcategory": {"type": "string"}
				},
				"required": ["category", "subcategory"]
			},
			"proposal": {
				"type": ["object", "null"],
				"properties": {
					"category": {"type": "string"},
					"subcategory": {"type": "string"},
					"icon": {"type": "string"},
					"description": {"type": "string"}
				},
				"required": ["category", "subcategory", "icon", "description"]
			}
		}
	}`),
}

// ClassifyCategoryCreate runs the CREATE_CATEGORY resolution: a semantic
// dup-check against the (already reserved/hidden-filtered) taxonomy, or a
// complete proposal the user only has to confirm. Exactly one of
// Match/Proposal is non-nil on success.
func (o *Orchestrator) ClassifyCategoryCreate(ctx context.Context, text string, taxonomy []TaxonomyEntry) (CategoryCreateResult, error) {
	systemPrompt := fmt.Sprintf(categoryCreateSystemPromptTemplate, buildTaxonomyBlock(taxonomy))

	raw, err := o.client.chatCompletion(ctx, callTypeCategoryCreate, o.createModel, systemPrompt, text, categoryCreateTool)
	if err != nil {
		return CategoryCreateResult{}, fmt.Errorf("orchestrator: classify category create: %w", err)
	}

	var result CategoryCreateResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return CategoryCreateResult{}, fmt.Errorf("orchestrator: parse category create result: %w", err)
	}
	if result.Match == nil && result.Proposal == nil {
		return CategoryCreateResult{}, fmt.Errorf("orchestrator: category create returned neither match nor proposal")
	}
	if result.Match != nil && result.Proposal != nil {
		// prefer the conservative answer when the model disobeys
		result.Proposal = nil
	}
	return result, nil
}
