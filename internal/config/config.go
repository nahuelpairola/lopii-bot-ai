package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

const (
	APP_ENV   = "env"
	ENV_LOCAL = "local"
	ENV_DEV   = "dev"
	ENV_PRD   = "prd"
)

type database struct {
	Host          string `mapstructure:"host"`
	Name          string `mapstructure:"name"`
	Port          uint   `mapstructure:"port"`
	User          string `mapstructure:"user"`
	Password      string `mapstructure:"password"`
	RunMigrations bool   `mapstructure:"runMigrations"`
}

type telegram struct {
	Username string `mapstructure:"username"`
	Token    string `mapstructure:"token"`
}

type server struct {
	Port     string `mapstructure:"port"`
	BaseHost string `mapstructure:"baseHost"`
}

type groq struct {
	APIKey      string `mapstructure:"apiKey"`
	BaseURL     string `mapstructure:"baseUrl"`
	CreateModel string `mapstructure:"createModel"`
	UpdateModel string `mapstructure:"updateModel"`
	QueryModel  string `mapstructure:"queryModel"`
	// AgentModel es el modelo del loop unificado (orchestrator.Run). Es un
	// sexto campo, no un reemplazo: los cinco por tipo de llamada siguen
	// sirviendo el camino viejo hasta la etapa 5.
	AgentModel string `mapstructure:"agentModel"`
	// AgentFallbackModels son los modelos a los que se corre el loop cuando el
	// principal rebota por CUPO, en orden. Los techos de Groq son por modelo, así
	// que un 429 en uno no dice nada del otro. Vacío = el 429 encola, como antes.
	AgentFallbackModels []string `mapstructure:"agentFallbackModels"`
	// QueryFallbackModels es lo mismo para el loop de consultas. Va aparte de la
	// del agente y NO reusa esa lista: el primario de query (120b) es justamente
	// el primer suplente del agente, así que reusarla haría que el primer
	// reintento cayera en el modelo que acaba de rebotar.
	QueryFallbackModels []string `mapstructure:"queryFallbackModels"`
	// NarrationModel es el modelo de la narración forzada de una consulta —la
	// última llamada, donde ya no se eligen herramientas y sólo se redacta—.
	// Redactar no es razonar: medido el 2026-08-13, un modelo razonador se come la
	// completion pensando y devuelve vacío. Vacío = se usa queryModel.
	NarrationModel string `mapstructure:"narrationModel"`
	// ClassifierModel es el modelo de la clasificación de categorías, que en la
	// etapa 5 sale del loop. Va en OTRO modelo a propósito: el techo de TPM de
	// Groq es por modelo, y medido el 2026-08-12 el loop ya entra al suyo una vez
	// por minuto y medio. Cuál conviene lo decide el eval, no este default.
	ClassifierModel string `mapstructure:"classifierModel"`
	TimeoutSeconds  int    `mapstructure:"timeoutSeconds"`
}

type query struct {
	HistoryTtlMinutes int `mapstructure:"historyTtlMinutes"`
	HistoryLimit      int `mapstructure:"historyLimit"`
}

type reminders struct {
	SweepIntervalMinutes int `mapstructure:"sweepIntervalMinutes"`
}

type logConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type Config struct {
	Env       string    `mapstructure:"env"`
	Server    server    `mapstructure:"server"`
	Database  database  `mapstructure:"database"`
	Telegram  telegram  `mapstructure:"telegram"`
	Groq      groq      `mapstructure:"groq"`
	Query     query     `mapstructure:"query"`
	Reminders reminders `mapstructure:"reminders"`
	Log       logConfig `mapstructure:"log"`
}

// applyDefaults carga los defaults sobre un Viper cualquiera, no sobre el singleton,
// para que un test pueda resolver un config file sin pisar el estado global.
func applyDefaults(v *viper.Viper) {
	v.SetDefault("Server.Port", "80")
	v.SetDefault("Query.HistoryTtlMinutes", 10)
	v.SetDefault("Query.HistoryLimit", 5)
	v.SetDefault("Reminders.SweepIntervalMinutes", 5)
	// Sin default, un entorno que no declare agentModel deja o.agentModel en ""
	// y Groq responde 400 en cada llamada del loop. Ya no hay camino alternativo:
	// desde la etapa 5, Run es el ÚNICO, así que un entorno nuevo sin este valor
	// no degrada, no arranca.
	v.SetDefault("Groq.AgentModel", "openai/gpt-oss-20b")
	// La cadena por default. Los tres soportan `tools` (verificado contra
	// /v1/models) y están ordenados por precio: gpt-oss-20b es el más barato de
	// los capaces, y llama-3.3-70b —el de mayor techo, 12.000 TPM— va último
	// porque su prompt cuesta 8 veces más. qwen queda AFUERA a propósito: su
	// completion sale $3 por millón, diez veces el 20b.
	v.SetDefault("Groq.AgentFallbackModels", []string{"openai/gpt-oss-120b", "openai/gpt-oss-20b"})
	// La cadena de query. llama-3.3-70b primero por el techo medido más alto
	// (12.000 TPM) y bucket propio; gpt-oss-20b último porque es el más barato pero
	// el más flojo narrando, y a esa altura la alternativa es no contestar.
	//
	// El orden de la cadena del AGENTE no se toca a propósito, aunque su primer
	// suplente (120b) sea el primario de query: ahora query tiene con qué correrse
	// de ese choque, e invertir el del agente mandaría todo el tráfico de rescate a
	// llama-3.3-70b, cuyo prompt cuesta ~8 veces más. Está último por precio.
	v.SetDefault("Groq.QueryFallbackModels", []string{"openai/gpt-oss-20b", "openai/gpt-oss-120b"})
	// openai/gpt-oss-20b no razona: narra en 35-61 tokens de completion contra
	// los 174-1.024 de gpt-oss, y por eso no puede quedarse sin presupuesto antes de
	// escribir. qwen queda afuera: emite su razonamiento DENTRO del contenido.
	// llama-3.1-8b-instant también: Groq lo da de baja el 2026-08-16.
	v.SetDefault("Groq.NarrationModel", "openai/gpt-oss-20b")
	// openai/gpt-oss-20b: el techo medido más alto (12.000 TPM), bucket
	// propio, y fuerte en español rioplatense. Punto de partida, no conclusión.
	v.SetDefault("Groq.ClassifierModel", "openai/gpt-oss-20b")
	v.SetDefault("Log.Level", "info")
	v.SetDefault("Log.Format", "json")
}

// sameTurnCalls son los pares de llamadas a Groq que pueden ocurrir en UN MISMO turno.
//
// Los techos de Groq (TPM, TPD) son POR MODELO, así que dos llamadas del mismo turno
// apuntando al mismo modelo compiten entre sí: la primera reserva y la segunda rebota.
// El 2026-08-13 costó ~95 segundos y ~11.000 tokens quemados en reintentos que no
// podían avanzar, porque `create` estaba en el mismo modelo que `agent`.
//
// Lo que NO entra, y es tan importante como lo que entra: `update` y `onboarding`
// viven en pasos de flow que llegan en mensajes POSTERIORES, no en el turno del
// agente. Competir entre turnos ya lo cubre la cadena de fallback.
//
// Tampoco entran `query`+`create` (hoy los dos en openai/gpt-oss-120b) ni
// `classifier`+`narration` (hoy los dos en openai/gpt-oss-20b), aunque en
// teoría podrían chocar: sólo aparecen si se lee la tabla como transitiva
// (agent-query más agent-create implicando query-create, y análogo para el otro
// par). No hay un trace que muestre a ninguno de los dos ocurriendo de verdad en
// el mismo turno —cada par que SÍ está listado, lo tiene—. Si algún día un trace
// muestra a alguno de estos dos pares chocando en producción, se agrega acá y se
// cambia de modelo.
//
// 2026-08-17: los modelos usables bajaron de tres a DOS. Groq dio de baja
// llama-3.1-8b-instant (2026-08-16) y también llama-3.3-70b-versatile, que era el
// que sostenía este invariante; quedan openai/gpt-oss-120b y openai/gpt-oss-20b,
// porque qwen/qwen3.6-27b emite su razonamiento adentro del contenido y deja la
// narración vacía. Con dos modelos, los cuatro pares de acá abajo NO se pueden
// satisfacer todos a la vez, así que TestEveryConfigFile_HasNoSameTurnModelCollision
// está rojo a propósito — el porqué completo, con las tres asignaciones medidas
// contra el eval real, está en el comentario de ese test. La tabla se deja intacta:
// describe qué choca de verdad, y esa verdad no cambió porque falte un modelo.
var sameTurnCalls = [][2]string{
	{"agent", "classifier"}, // ClassifyCategories sale del propio ejecutor del agente
	{"agent", "query"},      // el agente delega en answer_query dentro del mismo turno
	{"agent", "create"},     // el agente parkea en un wizard y el wizard clasifica
	// el agente delega en answer_query en el mismo turno, y query gasta cupo en
	// DOS modelos, no uno: el de las rondas de herramientas y el de la narración.
	{"agent", "narration"},
}

// ModelBucketConflicts devuelve una línea por cada par de sameTurnCalls que quedó
// apuntando al MISMO modelo. Vacío = config sana.
//
// Pura y sin I/O a propósito: se la puede correr sobre cualquier config resuelta, que
// es lo que hace el test sobre config/*.toml.
func ModelBucketConflicts(g groq) []string {
	narrationModel := g.NarrationModel
	if narrationModel == "" {
		narrationModel = g.QueryModel
	}
	modelOf := map[string]string{
		"agent":      g.AgentModel,
		"classifier": g.ClassifierModel,
		"query":      g.QueryModel,
		"create":     g.CreateModel,
		"narration":  narrationModel,
	}
	var out []string
	for _, par := range sameTurnCalls {
		a, b := modelOf[par[0]], modelOf[par[1]]
		if a == "" || b == "" || a != b {
			continue
		}
		out = append(out, fmt.Sprintf("%s y %s comparten el modelo %s, y pueden ocurrir en el mismo turno", par[0], par[1], a))
	}
	return out
}

func Initialize() (*Config, error) {
	env := os.Getenv(APP_ENV)
	configFilePath := fmt.Sprintf("../../config/%s.toml", env)
	viper.SetConfigFile(configFilePath)
	viper.SetConfigType("toml")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	applyDefaults(viper.GetViper())

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
