package main

import (
	"os"

	"lopiibot.com/src/config"
	"lopiibot.com/src/infrastructure/server"
)

func main() {
	cfg, err := config.Initialize()
	if err != nil {
		os.Exit(1)
	}
	if err := server.InitServer(cfg); err != nil {
		os.Exit(1)
	}
}
