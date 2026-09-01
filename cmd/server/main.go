package main

import (
	"fmt"
	"os"

	"lopiibot.com/internal/config"
	"lopiibot.com/internal/server"
)

func main() {
	cfg, err := config.Initialize()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	if err := server.InitServer(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}
