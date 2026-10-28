package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"price-crawler/services/data-finder/internal/config"
	"price-crawler/services/data-finder/internal/observability"
)

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	broker := &observability.Broker{URL: cfg.RabbitMQURL, InitialDelay: cfg.ReconnectInitial, MaxDelay: cfg.ReconnectMax}
	brokerCtx, stopBroker := context.WithCancel(ctx)
	defer stopBroker()
	firstReady := make(chan struct{})
	brokerDone := make(chan struct{})
	go func() { defer close(brokerDone); broker.Run(brokerCtx, firstReady) }()
	defer func() { stopBroker(); <-brokerDone }()
	startup, cancelStartup := context.WithTimeout(ctx, cfg.ReadinessTimeout)
	defer cancelStartup()
	if err := broker.WaitReady(startup, firstReady); err != nil {
		return err
	}

	// Business consumers are added with their corresponding workflow tasks.
	pool := observability.NewWorkerPool(cfg.WorkerConcurrency)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !broker.Ready() {
			http.Error(w, "RabbitMQ unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"alive"}`))
	})
	server := &http.Server{Addr: cfg.HTTPAddr, ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && !broker.Ready() {
			http.Error(w, "RabbitMQ unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := pool.Run(r.Context(), func(context.Context) error { mux.ServeHTTP(w, r); return nil }); err != nil {
			http.Error(w, "service is shutting down", http.StatusServiceUnavailable)
		}
	})}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("data-finder process started", "addr", cfg.HTTPAddr, "worker_concurrency", cfg.WorkerConcurrency)
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErr <- err
	}()
	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := observability.ShutdownHTTP(shutdown, server); err != nil {
		return fmt.Errorf("data-finder HTTP shutdown failed: %w", err)
	}
	return <-serverErr
}

func main() {
	logger := observability.Logger()
	cfg, err := config.Load()
	if err != nil {
		logger.Error("data-finder configuration failed", "error", err)
		os.Exit(1)
	}
	ctx, stop := observability.ShutdownContext(context.Background())
	defer stop()
	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("data-finder stopped", "error", err)
		os.Exit(1)
	}
}
