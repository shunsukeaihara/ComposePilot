package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"composepilot/internal/config"
	cryptox "composepilot/internal/crypto"
	httphandler "composepilot/internal/http"
	"composepilot/internal/store"
)

var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if cfg.ShowVersion {
		fmt.Println(version)
		return
	}
	cipher, err := cryptox.New(cfg.MasterKey)
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := httphandler.NewServer(cfg, st, cipher)
	if err := server.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
