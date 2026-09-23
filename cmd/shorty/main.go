package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	httpSrv := &http.Server{
		Addr:              cfg.Address,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown: stop accepting connections, then release
	// server-owned background resources (webhook dispatcher workers).
	// main waits for the shutdown goroutine so deferred db.Close() can't
	// race in-flight delivery writes.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
		srv.Close()
	}()

	log.Printf("shorty listening on %s", cfg.Address)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
	<-shutdownDone
}
