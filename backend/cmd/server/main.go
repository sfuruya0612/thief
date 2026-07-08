package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/api"
	"github.com/sfuruya0612/thief/backend/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	srv, err := api.NewServer(ctx, cfg)
	if err != nil {
		slog.Error("init server", "err", err)
		os.Exit(1)
	}
	defer srv.Close()

	httpSrv := srv.HTTPServer(ctx)
	slog.Info("server starting", "addr", httpSrv.Addr)

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil {
			slog.Info("server stopped", "err", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutdownCtx)
}
