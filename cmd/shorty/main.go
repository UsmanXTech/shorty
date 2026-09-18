package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/UsmanXTech/shorty/internal/analytics"
	"github.com/UsmanXTech/shorty/internal/config"
	"github.com/UsmanXTech/shorty/internal/database"
	"github.com/UsmanXTech/shorty/internal/geoip"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/server"
)

func main() {
	cfg := config.Load()

	db, err := database.Open(cfg.Database)
	if err != nil {
		log.Printf("database startup failed: %v", err)
		os.Exit(1)
	}
	defer db.Close()

	repo := links.NewSQLiteRepository(db.DB)
	recorder := analytics.New(analytics.NewSQLiteStore(db.DB), 256)
	defer recorder.Close(context.Background())

	var geo *geoip.Database
	if cfg.GeoIPDB != "" {
		geo, err = geoip.Open(cfg.GeoIPDB)
		if err != nil {
			log.Printf("geoip startup failed: %v", err)
			os.Exit(1)
		}
		log.Printf("local geoip database loaded from %s", cfg.GeoIPDB)
	}

	srv := server.NewWithRepositoryAndAnalyticsAndGeoIPAndDB(cfg, repo, recorder, geo, db.DB)

	log.Printf("shorty listening on %s", cfg.Address)
	if err := http.ListenAndServe(cfg.Address, srv.Handler()); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
