package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/termofeed/termofeed-back/internal/api"
	"github.com/termofeed/termofeed-back/internal/clock"
	"github.com/termofeed/termofeed-back/internal/config"
	"github.com/termofeed/termofeed-back/internal/ratelimit"
	"github.com/termofeed/termofeed-back/internal/room"
	"github.com/termofeed/termofeed-back/internal/store"
	"github.com/termofeed/termofeed-back/internal/ws"
)

func main() {
	// Subcommand: healthcheck (used for minimal container HEALTHCHECK)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		runHealthcheck()
		return
	}

	cfg := config.Load()

	// Configure structured slog JSON logger
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	logger.Info("starting termofeed backend",
		"port", cfg.Port,
		"public_base_url", cfg.PublicBaseURL,
		"trusted_proxy", cfg.TrustedProxy,
		"retention_hours", cfg.RetentionHours,
	)

	realClock := clock.NewRealClock()
	memStore := store.NewMemoryStore()
	hub := ws.NewHub()
	limiter := ratelimit.NewIPRateLimiter(realClock)
	roomService := room.NewService(memStore, realClock, hub, cfg.PublicBaseURL, cfg.RetentionHours)

	handler := api.NewHandler(roomService, hub, cfg, logger, prometheus.DefaultRegisterer)
	router := api.BuildRouter(handler, limiter)

	// Background ticker for room expiration, 60s warnings, and rate limit cleanup
	tickerCtx, cancelTicker := context.WithCancel(context.Background())
	defer cancelTicker()
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		cleanupCounter := 0
		for {
			select {
			case <-tickerCtx.Done():
				return
			case <-ticker.C:
				roomService.Tick(tickerCtx)
				cleanupCounter++
				if cleanupCounter%60 == 0 {
					limiter.Cleanup()
				}
			}
		}
	}()

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Server goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Graceful shutdown on SIGTERM / SIGINT
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Error("server startup error", "err", err)
		os.Exit(1)
	case sig := <-shutdown:
		logger.Info("shutdown signal received", "signal", sig.String())
	}

	logger.Info("initiating graceful shutdown (up to 10s)...")
	cancelTicker()

	// Close all WebSocket connections
	hub.CloseAll()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server forced shutdown error", "err", err)
		os.Exit(1)
	}

	logger.Info("termofeed backend stopped cleanly")
}

func runHealthcheck() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	url := fmt.Sprintf("http://127.0.0.1:%s/healthz", port)

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}
