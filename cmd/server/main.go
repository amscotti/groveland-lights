// Command server is the Groveland Lights web application.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amscotti/groveland-lights/internal/config"
	"github.com/amscotti/groveland-lights/internal/store"
	"github.com/amscotti/groveland-lights/internal/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	// Open the store before serving: this applies any pending migrations and
	// validates the recorded checksums. On a mismatch the error names the
	// offending migration file and both checksums — logging it here and
	// exiting non-zero is the boot refusal for a tampered or drifted DB.
	db, err := store.Open(context.Background(), cfg.DBPath)
	if err != nil {
		return err
	}
	// Close after the graceful shutdown has drained requests (defers run
	// last); the pool's final close checkpoints the WAL. A close error here
	// cannot be acted on, so it is deliberately discarded.
	defer func() { _ = db.Close() }()

	web.SetProbeStore(db)

	srv, err := web.New(cfg, logger)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort("0.0.0.0", cfg.Port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", addr, "env", cfg.AppEnv)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logger.Info("shutting down")
	return httpSrv.Shutdown(shutdownCtx)
}

func newLogger(cfg config.Config) *slog.Logger {
	if cfg.IsProd() {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, nil))
}
