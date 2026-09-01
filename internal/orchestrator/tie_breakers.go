package orchestrator

import "strings"

const tieBreakerRules = `- Monto o ítem solo → {{record}}.
- Reintegro/regalo que alude a algo previo → {{correct}}.
- Rendimiento de inversión → {{record}}.
- No confundir {{record}} con {{delete}}.
- Copulativo en pasado sobre un monto (era, eran, fue) → {{correct}}, aunque no diga "en realidad".
- Queja sobre un movimiento previo sin decir el valor nuevo ("estaba mal", "no era así") → {{correct}}: la app le pregunta qué cambiar.
- Ajustar/corregir el saldo o monto de una CUENTA → {{account}} ({{correct}} es solo sobre un movimiento).
- "¿Qué puedo hacer?" / "¿cómo funcionás?" → {{help}} (no {{query}}).`

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

func agentTieBreakers(available []AgentTool) string {
	rendered := renderTieBreakers(
		ToolRecordMovements,
		ToolCorrectMovement,
		ToolDeleteMovements,
		ToolManageSettings,
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
