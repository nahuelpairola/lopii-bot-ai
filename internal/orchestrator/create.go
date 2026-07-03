package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const createSystemPromptTemplate = `Sos un clasificador contable automatizado de alta precisión para un individuo en Argentina.
Tu tarea es convertir un mensaje en lenguaje natural en 1 o más movimientos financieros estructurados.

REGLAS DE TAXONOMÍA:
1. Usá ÚNICAMENTE las categorías y subcategorías listadas abajo. Prohibido inventar nombres nuevos.
2. Si tenés menos del 90%% de certeza sobre la categoría o subcategoría, asigná EXACTAMENTE "PENDING_REVIEW" en ambos campos.

REGLAS DE MONTO Y MONEDA:
- Los montos abreviados ("200k", "1.5m") se expanden a su valor numérico completo.
- Si el mensaje no aclara moneda, asumí ARS siempre.
- Método de pago por defecto si no se aclara: "transfer". Vocabulario sugerido: transfer, qr, cash, bank_deposit, check, broker, credit_card.

REGLA DE FECHA:
- Hoy es %s. Por defecto la fecha del movimiento es hoy. Si el mensaje aclara una fecha o día relativo ("el 3 de enero", "ayer", "el lunes pasado"), usá esa fecha. Sin año aclarado, asumí el año actual salvo que caiga en el futuro, en cuyo caso usá el año anterior. Todos los movimientos de un mismo mensaje comparten la misma fecha.

PATRONES DE MOVIMIENTOS COMPUESTOS (varios movimientos, comparten una misma transacción):
1. Compra de USD ("compré 100 usd a 1400"): expense en ARS por el total + transfer en USD hacia la cuenta indicada (o la de USD por defecto si no se aclara).
2. Suscripción a FCI ("suscribí a FCI 120k"): 2 transfers — afuera de la cuenta origen, adentro de la cuenta FCI. Nunca expense/income.
3. Rescate de FCI ("rescaté 100k de FCI"): 2 transfers — afuera de la cuenta FCI, adentro del destino. NO calcules ni agregues ganancia vos: nunca devuelvas un movimiento de income por un rescate, eso lo calcula la aplicación.
4. Itemización de tarjeta ("pago tarjeta 200k: ropa 50k, super 150k"): un expense por cada ítem nombrado, misma transacción. Si el mensaje aclara un total y dice que "el resto" es algo (ej. cargos de tarjeta), calculá ese resto vos mismo (total declarado menos la suma de los ítems nombrados) y agregalo como un expense más. Si la suma de los ítems nombrados supera el total declarado, no devuelvas ningún movimiento — es un error de datos del usuario.

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
						"amount": {"type": "string"},
						"currency": {"type": "string", "enum": ["ARS", "USD"]},
						"account_id": {"type": "integer"},
						"account_name_guess": {"type": "string"},
						"category": {"type": "string"},
						"subcategory": {"type": "string"},
						"payment_method": {"type": "string"},
						"merchant": {"type": "string"},
						"description": {"type": "string"},
						"date": {"type": "string"}
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
		lines = append(lines, fmt.Sprintf("%s | %s | %s", t.Category, t.Subcategory, t.Description))
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

	raw, err := o.client.chatCompletion(ctx, o.createModel, systemPrompt, text, createTool)
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
	return result, nil
}
