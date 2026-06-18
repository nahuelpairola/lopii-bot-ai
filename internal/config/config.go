package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"lopiibot.com/internal/constants"
)

type database struct {
	Host          string `mapstructure:"host"`
	Name          string `mapstructure:"name"`
	Port          uint   `mapstructure:"port"`
	User          string `mapstructure:"user"`
	Password      string `mapstructure:"password"`
	RunMigrations bool   `mapstructure:"runMigrations"`
}

type Config struct {
	Env      string   `mapstructure:"env"`
	Database database `mapstructure:"database"`
}

func Initialize() (*Config, error) {
	env := os.Getenv(constants.APP_ENV)
	configFilePath := fmt.Sprintf("../../config/%s.toml", env)
	viper.SetConfigFile(configFilePath)
	viper.SetConfigType("toml")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
