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
	RouterModel string `mapstructure:"routerModel"`
	CreateModel string `mapstructure:"createModel"`
	UpdateModel string `mapstructure:"updateModel"`
	DeleteModel string `mapstructure:"deleteModel"`
	QueryModel  string `mapstructure:"queryModel"`
	// AgentModel es el modelo del loop unificado (orchestrator.Run). Es un
	// sexto campo, no un reemplazo: los cinco por tipo de llamada siguen
	// sirviendo el camino viejo hasta la etapa 5.
	AgentModel     string `mapstructure:"agentModel"`
	TimeoutSeconds int    `mapstructure:"timeoutSeconds"`
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
	// y Groq responde 400 en cada llamada del loop. Hoy es inofensivo porque
	// nadie llama a Run, pero desde la etapa 2 seria una mina para cualquier
	// entorno nuevo.
	viper.SetDefault("Groq.AgentModel", "openai/gpt-oss-20b")
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
