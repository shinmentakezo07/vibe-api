// Command vibe-api serves an OpenAI-compatible endpoint backed by Mistral.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vibe-api/internal/config"
	"vibe-api/internal/keys"
	"vibe-api/internal/mistral"
	"vibe-api/internal/server"
	"vibe-api/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("configuration", "error", err)
		os.Exit(1)
	}

	upstream := mistral.New(cfg.BaseURL, cfg.UserAgent)

	var (
		db         *store.Postgres
		initial    []keys.Key
		roundRobin bool
	)
	if cfg.DatabaseURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		db, err = store.Open(ctx, cfg.DatabaseURL, cfg.AppSecret, log)
		cancel()
		if err != nil {
			log.Error("database", "error", err)
			os.Exit(1)
		}
		defer db.Close()
		log.Info("postgres connected")

		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		initial, err = db.ListAPIKeys(ctx)
		if err == nil {
			roundRobin, err = db.LoadRoundRobin(ctx)
		}
		cancel()
		if err != nil {
			log.Error("load API keys", "error", err)
			os.Exit(1)
		}
		log.Info("loaded API keys", "count", len(initial))
	}

	if cfg.APIKey != "" {
		present := false
		for _, key := range initial {
			if key.Secret == cfg.APIKey {
				present = true
				break
			}
		}
		if !present {
			initial = append(initial, keys.Key{
				ID:        "env",
				Label:     "Environment",
				Secret:    cfg.APIKey,
				Enabled:   true,
				CreatedAt: time.Now(),
				Source:    "environment",
			})
			log.Info("environment API key added to the pool")
		}
	}

	keyring := keys.New(initial, roundRobin)
	if keyring.EnabledCount() == 0 {
		log.Warn("no enabled API key configured; add one from the dashboard")
	}
	handler := server.New(cfg, upstream, db, keyring, log).Router()
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	log.Info("listening", "addr", cfg.ListenAddr, "upstream", cfg.BaseURL)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-stop:
		log.Info("shutting down", "signal", sig.String())
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server", "error", err)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown", "error", err)
	}
}
