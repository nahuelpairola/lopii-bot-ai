//go:build llm_eval

package orchestrator

import (
	"os"
	"testing"
)

// evalKey devuelve la API key de Groq, o corta el test si no hay.
//
// Corta, no saltea: el tag `llm_eval` se pide a mano, así que quien corrió esto
// quiso gastar cuota contra el modelo real. Un skip ahí es un verde que no
// probó nada, y el verde es exactamente lo que se mira.
//
// El nombre canónico es GROQ_APIKEY — es el que está en `.env` y el que Viper
// deriva de `groq.apiKey`. Estos evals leían GROQ_API_KEY, que no lo exporta
// nadie, así que se salteaban SIEMPRE, incluso siguiendo la doc al pie de la
// letra. Se acepta el alias viejo sólo porque está en los comandos de ejemplo
// al principio de cada archivo de eval.
func evalKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("GROQ_APIKEY")
	if key == "" {
		key = os.Getenv("GROQ_API_KEY")
	}
	if key == "" {
		t.Fatal("GROQ_APIKEY unset — the llm_eval tag was requested on purpose, so skipping would be a green that proves nothing. Export it from .env.")
	}
	return key
}

// evalBaseURL es la URL de Groq. Default y no fatal: la baseUrl no vive en el
// .env sino en el TOML (groq.baseUrl), así que exigirla por entorno hacía que
// el eval muriera con "unsupported protocol scheme" — el mismo modo de falla
// que el GROQ_API_KEY que nadie exportaba.
func evalBaseURL() string {
	if u := os.Getenv("GROQ_BASE_URL"); u != "" {
		return u
	}
	return "https://api.groq.com/openai/v1"
}
