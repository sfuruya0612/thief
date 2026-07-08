package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/api"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/spf13/cobra"
)

func newServerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "Start the API server (127.0.0.1:8080)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			srv, err := api.NewServer(ctx, cfg)
			if err != nil {
				return err
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
			return httpSrv.Shutdown(shutdownCtx)
		},
	}
}
