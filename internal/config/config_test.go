package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// El invariante, sobre lo que REALMENTE se deploya.
//
// No alcanza con chequear los defaults: createModel, updateModel y queryModel no
// tienen default y salen sólo del TOML, así que un test sobre defaults vería "" y
// pasaría en verde sin probar nada. El choque vive en el archivo.
//
// Recorre el directorio en vez de una lista fija para que un entorno nuevo quede
// cubierto sin que nadie se acuerde de agregarlo.
//
// ESTE TEST ESTÁ ROJO A PROPÓSITO desde el 2026-08-17, y se deja rojo por decisión
// explícita. No lo "arregles" cambiando modelos sin leer esto:
//
// Groq dio de baja llama-3.3-70b-versatile (404 model_not_found). Con él se caen a
// DOS los modelos usables — openai/gpt-oss-120b y openai/gpt-oss-20b; qwen/qwen3.6-27b
// no sirve porque emite su razonamiento adentro del contenido y la narración vuelve
// vacía (medido). Y sameTurnCalls pide que `agent` no comparta modelo con `classifier`,
// `query`, `create` NI `narration`: con dos modelos, los otros cuatro tienen que ir
// todos en el que no es el del agente. Las tres asignaciones posibles se midieron
// contra el eval real (`-tags query_eval`):
//
//   - narration/classifier en 20b (lo de hoy, y lo que corre en prod): eval VERDE,
//     este test rojo. Los logs de Render muestran la consecuencia real: el
//     "modelo sin cupo, probando el siguiente" que se repite desde el deploy del 15/08.
//   - narration/classifier en 120b: este test verde, pero el eval falla 4 de 4 en
//     "filtro_inexistente" — el bot narra "gastaste $0" sobre una categoría que NO
//     EXISTE. Mentirle al usuario es peor que rebotar contra el cupo.
//   - query+narration en 20b: rompe "saldos" y "cuenta_inexistente".
//
// O sea: no hay config que deje verdes al test y al eval a la vez. Se eligió el eval.
// El rojo queda como recordatorio de que falta un tercer bucket de TPM (otro tier u
// otro proveedor); el día que exista, esto vuelve solo a verde.
func TestEveryConfigFile_HasNoSameTurnModelCollision(t *testing.T) {
	archivos, err := filepath.Glob(filepath.Join("..", "..", "config", "*.toml"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(archivos) == 0 {
		t.Fatal("no se encontró ningún config/*.toml: el test no está mirando nada")
	}
	for _, ruta := range archivos {
		t.Run(filepath.Base(ruta), func(t *testing.T) {
			v := viper.New()
			v.SetConfigFile(ruta)
			v.SetConfigType("toml")
			applyDefaults(v)
			if err := v.ReadInConfig(); err != nil {
				t.Fatalf("leer %s: %v", ruta, err)
			}
			var cfg Config
			if err := v.Unmarshal(&cfg); err != nil {
				t.Fatalf("unmarshal %s: %v", ruta, err)
			}
			if conflictos := ModelBucketConflicts(cfg.Groq); len(conflictos) > 0 {
				t.Errorf("%s tiene llamadas del mismo turno compartiendo modelo:\n  %s",
					ruta, strings.Join(conflictos, "\n  "))
			}
		})
	}
}

// Que la función detecte algo: sin esto, una que devuelva siempre vacío pasaría el
// test de arriba.
func TestModelBucketConflicts_DetectsACollision(t *testing.T) {
	g := groq{
		AgentModel:      "modelo-x",
		CreateModel:     "modelo-x",
		QueryModel:      "modelo-y",
		ClassifierModel: "modelo-z",
	}
	conflictos := ModelBucketConflicts(g)
	if len(conflictos) != 1 {
		t.Fatalf("conflictos = %v, want exactamente 1 (agent y create)", conflictos)
	}
	if !strings.Contains(conflictos[0], "agent") || !strings.Contains(conflictos[0], "create") {
		t.Errorf("el conflicto tiene que nombrar los dos lados: %q", conflictos[0])
	}
}

// Dos llamadas que NO comparten turno pueden compartir modelo sin problema: competir
// entre turnos ya lo cubre la cadena de fallback. Si esto se reporta, la tabla está
// de más y el warning se vuelve ruido que se ignora.
func TestModelBucketConflicts_IgnoresCallsThatNeverShareATurn(t *testing.T) {
	g := groq{
		AgentModel:      "a",
		CreateModel:     "compartido",
		UpdateModel:     "compartido",
		QueryModel:      "q",
		ClassifierModel: "c",
	}
	if conflictos := ModelBucketConflicts(g); len(conflictos) != 0 {
		t.Errorf("update y create no ocurren en el mismo turno: %v", conflictos)
	}
}
