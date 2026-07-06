package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NPetersenDK/eBot-CSGO-API/internal/api"
	"github.com/NPetersenDK/eBot-CSGO-API/internal/config"
	"github.com/NPetersenDK/eBot-CSGO-API/internal/db"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(cfg.DSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(database, cfg.APIKey),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("eBot-CSGO API listening on %s (swagger at /swagger/)", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
