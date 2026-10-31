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
	_ "time/tzdata" // Alpine does not ship zoneinfo; DISPLAY_TZ still has to resolve.

	"glucose-dashboard/internal/api"
	"glucose-dashboard/internal/config"
	"glucose-dashboard/internal/store"
	"glucose-dashboard/internal/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration", "err", err)
		os.Exit(1)
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		slog.Error("database open failed")
		os.Exit(1)
	}
	defer st.Close()

	page, err := web.NewHandler(st, cfg.DisplayTZ, cfg.TargetLow, cfg.TargetHigh)
	if err != nil {
		slog.Error("page setup failed")
		os.Exit(1)
	}

	srv := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: api.NewMux(&api.Server{
			Store:  st,
			APIKey: cfg.APIKey,
		}, cfg.DashPassword, page),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.ListenAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			slog.Error("shutdown", "err", err)
			os.Exit(1)
		}
	}
}
