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
//
// available son las tools que se van a mandar en el request. Una regla que
// nombra una tool ausente se descarta ENTERA: decirle al modelo "para esto usá
// record_movements" cuando record_movements no viaja en el request es pedirle
// que llame algo que no existe. Durante las etapas 2 y 3 el toolbox es un
// subconjunto, así que esto no es hipotético.
//
// Con las 14 no descarta nada, que es lo que va a pasar a partir de la etapa 4.
func agentTieBreakers(available []AgentTool) string {
	rendered := renderTieBreakers(
		ToolRecordMovements,
		ToolCorrectMovement,
		ToolDeleteMovements,
		ToolManageAccount,
		ToolReplyHelp,
		ToolSumMovements,
	)

	have := make(map[string]bool, len(available))
	for _, t := range available {
		have[t.Name] = true
	}
	all := AgentTools()

	kept := make([]string, 0, strings.Count(rendered, "\n")+1)
	for _, line := range strings.Split(rendered, "\n") {
		complete := true
		for _, t := range all {
			if strings.Contains(line, t.Name) && !have[t.Name] {
				complete = false
				break
			}
		}
		if complete {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}
