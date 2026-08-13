package orchestrator

import (
	"fmt"
	"strings"
)

// Los dos bloques que arma la app para cualquier prompt que necesite las
// cuentas o la taxonomía del usuario.
//
// Vivían en create.go, que era el camino pre-loop de CREATE. Ese camino murió
// con la etapa 5 (el router ya no existe y nadie llama ClassifyCreate), pero
// estos dos helpers los usan el prompt del agente, el clasificador,
// account_manage, category_create y update — así que salen a un archivo propio
// en vez de irse con el borrado.

// buildTaxonomyBlock renderiza los pares con su descripción.
//
// Una descripción vacía es deliberada, no un dato faltante: la migración de
// podado vacía las notas que no desambiguan nada, así el bloque solo paga
// tokens por las que deciden algo. Renderizar "Cat | Sub | " gastaría el
// separador al pedo y el modelo lo lee como una nota que quedó cortada.
func buildTaxonomyBlock(taxonomy []TaxonomyEntry) string {
	lines := make([]string, 0, len(taxonomy))
	for _, t := range taxonomy {
		line := fmt.Sprintf("%s | %s", t.Category, t.Subcategory)
		if t.Description != "" {
			line += " | " + t.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// buildAccountsBlock renderiza "id | nombre (moneda)". El id va porque el
// modelo tiene que poder devolver una cuenta EXISTENTE por id y no por nombre:
// un nombre lo puede escribir mal, un id no.
func buildAccountsBlock(accounts []AccountOption) string {
	lines := make([]string, 0, len(accounts))
	for _, a := range accounts {
		lines = append(lines, fmt.Sprintf("%d | %s (%s)", a.ID, a.Name, a.Currency))
	}
	return strings.Join(lines, "\n")
}
