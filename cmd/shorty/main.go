package main

import (
	"log"
	"net/http"
	"os"

	"github.com/UsmanXTech/shorty/internal/config"
	"github.com/UsmanXTech/shorty/internal/server"
)

func main() {
	cfg := config.Load()

	srv := server.New(cfg)
	log.Printf("shorty listening on %s", cfg.Address)

	if err := http.ListenAndServe(cfg.Address, srv.Handler()); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
