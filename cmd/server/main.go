package main

import (
	"os"

	"lopiibot.com/internal/config"
	"lopiibot.com/internal/server"
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
