package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"lopiibot.com/internal/config"
	"lopiibot.com/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	cfg, err := config.Initialize()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	if err := server.InitServer(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
}
