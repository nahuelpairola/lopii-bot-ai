package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
)

const onboardingSystemPrompt = `Sos un asistente que ayuda a una persona en Argentina a describir sus cuentas al arrancar.
Convertí el mensaje en lenguaje natural en una lista de cuentas, cada una con nombre, moneda y saldo actual.

REGLAS:
- Extraé SOLO un saldo monetario (un único número en ARS o USD) por cuenta. NUNCA unidades, acciones, tickers ni cantidades (ej. "100 cedears", "5000 cuotapartes" NO son saldos). Si el usuario da una posición en unidades, no la conviertas ni inventes una valuación: omití esa cuenta.
- Los montos con palabras se expanden a su valor numérico: "10 mil" → 10000, "1 millón" → 1000000, "100.000" → 100000, "200k" → 200000, "25 lucas" → 25000.
- Si no se aclara la moneda de una cuenta, asumí ARS. "pesos" → ARS; "dólares"/"USD"/"verdes" → USD.
- Si una cuenta no tiene nombre, llamala "Efectivo".
- Puede haber varias cuentas de la misma moneda (ej. dos en ARS). Cada una lleva su propio saldo.
- Si el mensaje no describe ninguna cuenta con saldo, devolvé una lista vacía.`

var onboardingTool = toolSchema{
	Name:        "describe_accounts",
	Description: "Registra las cuentas y saldos que el usuario describió al arrancar",
	Parameters: json.RawMessage(`{
		"type": "object",
		"properties": {
			"accounts": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"name": {"type": ["string", "null"]},
						"currency": {"type": "string", "enum": ["ARS", "USD"]},
						"balance": {"type": "string"}
					},
					"required": ["currency", "balance"]
				}
			}
		},
		"required": ["accounts"]
	}`),
}

func (o *Orchestrator) ClassifyOnboarding(ctx context.Context, text string) (OnboardingResult, error) {
	raw, err := o.client.chatCompletion(ctx, callTypeOnboarding, o.createModel, onboardingSystemPrompt, text, onboardingTool)
	if err != nil {
		return OnboardingResult{}, fmt.Errorf("orchestrator: classify onboarding: %w", err)
	}
	var result OnboardingResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return OnboardingResult{}, fmt.Errorf("orchestrator: parse onboarding result: %w", err)
	}
	return result, nil
}
