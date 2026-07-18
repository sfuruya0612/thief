package cli

import (
	"context"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
	"github.com/sfuruya0612/thief/backend/internal/config"
	"github.com/sfuruya0612/thief/backend/internal/util"
	"github.com/spf13/cobra"
)

var cwLogsGroupColumns = []util.Column{
	{Header: "Name"},
	{Header: "StoredBytes"},
	{Header: "RetentionDays"},
	{Header: "CreationTime"},
}

func newLogsCmd() *cobra.Command {
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Manage CloudWatch Logs resources",
		Long:  `Provides commands to list CloudWatch Logs log groups.`,
	}

	// ロググループ一覧の取得のみ対応する。イベント検索は複数グループ横断・ページング、
	// Live Tail は WebSocket 前提のため CLI スコープ外とする (GCP logging と同方針)。
	lsCmd := &cobra.Command{
		Use:   "ls",
		Short: "List log groups",
		Long:  `Retrieves and displays all CloudWatch Logs log groups.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd, ListConfig[awsinternal.LogGroupInfo]{
				Columns:  cwLogsGroupColumns,
				EmptyMsg: "No log groups found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.LogGroupInfo, error) {
					return awsinternal.ListLogGroups(ctx, cfg.Profile, cfg.Region)
				},
			})
		},
	}

	logsCmd.AddCommand(lsCmd)
	return logsCmd
}
