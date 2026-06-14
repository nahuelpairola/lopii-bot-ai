package main

import (
	"os"

	"lopiibot.com/src/config"
	"lopiibot.com/src/infrastructure/server"
)

func main() {
	configFilePath := "./config/local.toml"
	cfg, err := config.LoadConfigFrom(configFilePath)
	if err != nil {
		os.Exit(1)
	}
	if err := server.InitServer(cfg); err != nil {
		os.Exit(1)
	}
}
