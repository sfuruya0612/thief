package cli

import (
	"context"
	"log/slog"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/api"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/spf13/cobra"
)

// serverShutdownTimeout は待ち受け中のリクエストの完了を待つ猶予。
// 中断でコマンドの context がキャンセル済みでも猶予を与えたいため、専用の context を
// 作る際の期限として使う。
const serverShutdownTimeout = 5 * time.Second

func newServerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "Start the API server (127.0.0.1:8089)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// シグナル連動のキャンセルは main が 1 箇所で設定する。ここで自前の
			// signal.NotifyContext を張ると同じシグナルを二重に購読することになる。
			ctx := commandContext(cmd)

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
			// ctx はここへ来た時点でキャンセル済みなので、シャットダウンの猶予は
			// Background から派生させる。ctx から派生させると即座に期限切れになる。
			shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
			defer cancel()
			return httpSrv.Shutdown(shutdownCtx)
		},
	}
}
