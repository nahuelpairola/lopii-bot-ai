package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Los bloques compartidos con el prompt del loop viven en movement_rules.go.
// REGLA DE FECHA queda acá porque la del loop dice otra cosa.
const createSystemPromptTemplate = `Sos un clasificador contable automatizado de alta precisión para un individuo en Argentina.
Tu tarea es convertir un mensaje en lenguaje natural en 1 o más movimientos financieros estructurados.
` + taxonomyAndAmountRules + `
REGLA DE FECHA:
- Hoy es %s. Por defecto la fecha del movimiento es hoy. Si el mensaje aclara una fecha o día relativo ("el 3 de enero", "ayer", "el lunes pasado"), usá esa fecha. Sin año aclarado, asumí el año actual salvo que caiga en el futuro, en cuyo caso usá el año anterior. Todos los movimientos de un mismo mensaje comparten la misma fecha.
` + movementPatternRules + `
CUENTAS DEL USUARIO (id | nombre (moneda)):
%s

TAXONOMÍA DISPONIBLE (categoría | subcategoría | descripción):
%s`

var createTool = toolSchema{
	Name:        "record_movements",
	Description: "Registra uno o más movimientos financieros a partir del mensaje del usuario",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"movements": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"type": {"type": "string", "enum": ["expense", "income", "transfer"]},
						"amount": {"type": "string", "description": "positivo, EXCEPTO la pierna de un transfer que sale de una cuenta: esa va negativa. Las 2 piernas de un transfer suman 0"},
						"currency": {"type": "string", "enum": ["ARS", "USD"]},
						"account_id": {"type": ["integer", "null"]},
						"account_name_guess": {"type": ["string", "null"]},
						"category": {"type": "string"},
						"subcategory": {"type": "string"},
						"payment_method": {"type": "string"},
						"merchant": {"type": ["string", "null"]},
						"description": {"type": "string"},
						"date": {"type": "string"},
						"group": {"type": ["string", "null"]}
					},
					"required": ["type", "amount", "currency", "category", "subcategory", "payment_method", "description", "date"]
				}
			}
		},
		"required": ["movements"]
	}`),
}

func buildTaxonomyBlock(taxonomy []TaxonomyEntry) string {
	lines := make([]string, 0, len(taxonomy))
	for _, t := range taxonomy {
		// Una descripción vacía es deliberada, no un dato faltante: la migración de
		// podado vacía las notas que no desambiguan nada, así el bloque solo paga
		// tokens por las que deciden algo. Renderizar "Cat | Sub | " gastaría el
		// separador al pedo y el modelo lo lee como una nota que quedó cortada.
		line := fmt.Sprintf("%s | %s", t.Category, t.Subcategory)
		if t.Description != "" {
			line += " | " + t.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func buildAccountsBlock(accounts []AccountOption) string {
	lines := make([]string, 0, len(accounts))
	for _, a := range accounts {
		lines = append(lines, fmt.Sprintf("%d | %s (%s)", a.ID, a.Name, a.Currency))
	}
	return strings.Join(lines, "\n")
}

func (o *Orchestrator) ClassifyCreate(ctx context.Context, text string, taxonomy []TaxonomyEntry, accounts []AccountOption, today string) (CreateResult, error) {
	systemPrompt := fmt.Sprintf(createSystemPromptTemplate, today, buildAccountsBlock(accounts), buildTaxonomyBlock(taxonomy))

	raw, err := o.client.chatCompletion(ctx, callTypeCreate, o.createModel, systemPrompt, text, createTool)
	if err != nil {
		return CreateResult{}, fmt.Errorf("orchestrator: classify create: %w", err)
	}

	var result CreateResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return CreateResult{}, fmt.Errorf("orchestrator: parse create result: %w", err)
	}
	if len(result.Movements) == 0 {
		return CreateResult{}, fmt.Errorf("orchestrator: create result had no movements")
	}
	result.Normalize()
	return result, nil
}
