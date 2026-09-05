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
	"github.com/NPetersenDK/eBot-CSGO-API/internal/ebotcmd"
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

	// Optional: without Redis the API keeps working, minus live control.
	var cmd *ebotcmd.Publisher
	if cfg.RedisAddr != "" {
		cmd = ebotcmd.NewPublisher(cfg.RedisAddr, cfg.RedisUsername, cfg.RedisPassword, cfg.RedisList)
		defer cmd.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := cmd.Ping(ctx); err != nil {
			log.Printf("WARN redis %s unreachable, live control will fail: %v", cfg.RedisAddr, err)
		}
		cancel()
	} else {
		log.Print("REDIS_HOST unset: match stop/restart disabled")
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.New(database, cfg.APIKey, cmd),
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
