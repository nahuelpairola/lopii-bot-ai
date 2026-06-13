package config

import (
	"errors"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Env string `toml:"env"`
}

var config Config

func LoadConfigFrom(tomlFilePath string) (*Config, error) {
	if !fileExists(tomlFilePath) {
		return nil, errors.New("file path does not exists to load the config, check it " + tomlFilePath)
	}
	content, err := os.ReadFile(tomlFilePath)
	if err != nil {
		return nil, err
	}
	_, err = toml.Decode(string(content), &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}
