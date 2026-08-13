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

func Initialize() (*Config, error) {
	env := os.Getenv(APP_ENV)
	configFilePath := fmt.Sprintf("../../config/%s.toml", env)
	viper.SetConfigFile(configFilePath)
	viper.SetConfigType("toml")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetDefault("Server.Port", "80")
	viper.SetDefault("Query.HistoryTtlMinutes", 10)
	viper.SetDefault("Query.HistoryLimit", 5)
	viper.SetDefault("Reminders.SweepIntervalMinutes", 5)
	// Sin default, un entorno que no declare agentModel deja o.agentModel en ""
	// y Groq responde 400 en cada llamada del loop. Ya no hay camino alternativo:
	// desde la etapa 5, Run es el ÚNICO, así que un entorno nuevo sin este valor
	// no degrada, no arranca.
	viper.SetDefault("Groq.AgentModel", "openai/gpt-oss-20b")
	// La cadena por default. Los tres soportan `tools` (verificado contra
	// /v1/models) y están ordenados por precio: gpt-oss-20b es el más barato de
	// los capaces, y llama-3.3-70b —el de mayor techo, 12.000 TPM— va último
	// porque su prompt cuesta 8 veces más. qwen queda AFUERA a propósito: su
	// completion sale $3 por millón, diez veces el 20b.
	viper.SetDefault("Groq.AgentFallbackModels", []string{"openai/gpt-oss-120b", "llama-3.3-70b-versatile"})
	// La cadena de query. llama-3.3-70b primero por el techo medido más alto
	// (12.000 TPM) y bucket propio; gpt-oss-20b último porque es el más barato pero
	// el más flojo narrando, y a esa altura la alternativa es no contestar.
	//
	// El orden de la cadena del AGENTE no se toca a propósito, aunque su primer
	// suplente (120b) sea el primario de query: ahora query tiene con qué correrse
	// de ese choque, e invertir el del agente mandaría todo el tráfico de rescate a
	// llama-3.3-70b, cuyo prompt cuesta ~8 veces más. Está último por precio.
	viper.SetDefault("Groq.QueryFallbackModels", []string{"llama-3.3-70b-versatile", "openai/gpt-oss-20b"})
	// llama-3.3-70b-versatile: el techo medido más alto (12.000 TPM), bucket
	// propio, y fuerte en español rioplatense. Punto de partida, no conclusión.
	viper.SetDefault("Groq.ClassifierModel", "llama-3.3-70b-versatile")
	viper.SetDefault("Log.Level", "info")
	viper.SetDefault("Log.Format", "json")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
