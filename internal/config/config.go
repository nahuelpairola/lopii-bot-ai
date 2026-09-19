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
	GinMode  string `mapstructure:"ginMode"`
}

type groq struct {
	APIKey              string   `mapstructure:"apiKey"`
	BaseURL             string   `mapstructure:"baseUrl"`
	CreateModel         string   `mapstructure:"createModel"`
	UpdateModel         string   `mapstructure:"updateModel"`
	QueryModel          string   `mapstructure:"queryModel"`
	AgentModel          string   `mapstructure:"agentModel"`
	AgentFallbackModels []string `mapstructure:"agentFallbackModels"`
	QueryFallbackModels []string `mapstructure:"queryFallbackModels"`
	NarrationModel      string   `mapstructure:"narrationModel"`
	ClassifierModel     string   `mapstructure:"classifierModel"`
	TimeoutSeconds      int      `mapstructure:"timeoutSeconds"`
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

func applyDefaults(v *viper.Viper) {
	v.SetDefault("Server.Port", "80")
	v.SetDefault("Query.HistoryTtlMinutes", 10)
	v.SetDefault("Query.HistoryLimit", 5)
	v.SetDefault("Reminders.SweepIntervalMinutes", 5)
	v.SetDefault("Groq.AgentModel", "openai/gpt-oss-20b")
	v.SetDefault("Groq.AgentFallbackModels", []string{"openai/gpt-oss-120b", "qwen/qwen3.8-27b"})
	v.SetDefault("Groq.QueryFallbackModels", []string{"qwen/qwen3.8-27b", "openai/gpt-oss-20b"})
	v.SetDefault("Groq.NarrationModel", "openai/gpt-oss-20b")
	v.SetDefault("Groq.ClassifierModel", "openai/gpt-oss-20b")
	v.SetDefault("Log.Level", "info")
	v.SetDefault("Log.Format", "json")
}

var sameTurnCalls = [][2]string{
	{"agent", "classifier"},
	{"agent", "query"},
	{"agent", "create"},
	{"agent", "narration"},
}

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
