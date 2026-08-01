package orchestrator

import "strings"

// tieBreakerRules son los desempates que decidieron, mensaje real por mensaje
// real, a dónde va un texto ambiguo. Viven acá y en un solo lugar porque los
// necesitan DOS prompts a la vez durante las etapas 2 a 4 del agent loop:
//
//   - routerSystemPrompt, que sigue vivo y elige un INTENT
//   - agentSystemPromptTemplate, que elige una TOOL
//
// Duplicados serían una bomba de tiempo: cualquiera que tunee el router en los
// próximos meses dejaría el prompt unificado desincronizado en silencio, y
// nadie se enteraría hasta la etapa 4.
//
// Los marcadores se reemplazan por el vocabulario de cada prompt. La REGLA es
// la misma; lo único que cambia es cómo se llama la salida.
const tieBreakerRules = `- Monto o ítem solo → {{record}}.
- Reintegro/regalo que alude a algo previo → {{correct}}.
- Rendimiento de inversión → {{record}}.
- No confundir {{record}} con {{delete}}.
- Copulativo en pasado sobre un monto (era, eran, fue) → {{correct}}, aunque no diga "en realidad".
- Ajustar/corregir el saldo o monto de una CUENTA → {{account}} ({{correct}} es solo sobre un movimiento).
- "¿Qué puedo hacer?" / "¿cómo funcionás?" → {{help}} (no {{query}}).`

// renderTieBreakers instancia las reglas con los nombres que usa cada prompt.
func renderTieBreakers(record, correct, del, account, help, query string) string {
	return strings.NewReplacer(
		"{{record}}", record,
		"{{correct}}", correct,
		"{{delete}}", del,
		"{{account}}", account,
		"{{help}}", help,
		"{{query}}", query,
	).Replace(tieBreakerRules)
}

// routerTieBreakers es el bloque tal cual lo venía llevando el router. El texto
// renderizado tiene que quedar idéntico al que estaba escrito a mano: es un
// prompt tuneado contra producción y este refactor no puede moverle una coma.
// Lo fija TestRouterTieBreakers_TextIsUnchanged.
func routerTieBreakers() string {
	return renderTieBreakers("CREATE", "UPDATE", "DELETE", "ACCOUNT_MANAGE", "HELP", "QUERY")
}

// agentTieBreakers son las mismas reglas en el vocabulario de tools del loop
// unificado.
func agentTieBreakers() string {
	return renderTieBreakers(
		ToolRecordMovements,
		ToolCorrectMovement,
		ToolDeleteMovements,
		ToolManageAccount,
		ToolReplyHelp,
		ToolSumMovements,
	)
}
