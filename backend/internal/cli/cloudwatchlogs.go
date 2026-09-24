package cli

import (
	"context"
	"fmt"
	"time"

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

// cwLogsEventColumns は `thief logs events` の列定義。Severity はイベントが構造化された
// severity を持たないため、メッセージ本文から推定した 3 段階 (LogEventInfo.ToRow を参照)。
var cwLogsEventColumns = []util.Column{
	{Header: "Timestamp"},
	{Header: "Severity"},
	{Header: "LogStream"},
	{Header: "Message"},
}

// cwLogsDefaultEventLimit は `thief logs events` の既定取得件数。backend の
// 1 ロググループ 1 ページあたりの上限 (internal/aws の defaultLogEventPerGroupLimit) に揃える。
const cwLogsDefaultEventLimit = 100

// cwLogsMaxEventLimit は FilterLogEvents の Limit が受け付ける最大値 (API リファレンスの
// Valid Range: 1 - 10000)。取得層は int32 へ変換して API に渡すため、ここで範囲を検査しないと
// 範囲外の値が黙って折り返されたり API のエラーになったりする。
const cwLogsMaxEventLimit = 10000

// cwLogsEventsRequest は `thief logs events` の取得条件。フラグと位置引数から組み立てる。
type cwLogsEventsRequest struct {
	LogGroup string
	Filter   string
	Since    time.Duration
	Limit    int
}

// cwLogsFilterEventsFunc は awsinternal.FilterLogEvents と同じシグネチャ。テストで差し替える。
type cwLogsFilterEventsFunc func(ctx context.Context, profile, region string, groupIdentifiers []string, pattern, start, end, pageToken string, perGroupLimit int) (*awsinternal.LogEventPage, error)

// cwLogsEventsRequestFromFlags は events サブコマンドのフラグと位置引数から取得条件を組み立てる。
// --since は 0 以下を、--limit は 1 から cwLogsMaxEventLimit の範囲外を受け付けない。backend の
// FilterLogEvents は 0 以下の上限を黙って既定値へ置き換え、範囲を超える値は int32 変換で折り返す
// ことがあるため、CLI では利用者の指定ミスをここで止める。
func cwLogsEventsRequestFromFlags(cmd *cobra.Command, logGroup string) (cwLogsEventsRequest, error) {
	filter, _ := cmd.Flags().GetString("filter")
	since, _ := cmd.Flags().GetDuration("since")
	limit, _ := cmd.Flags().GetInt("limit")
	if since <= 0 {
		return cwLogsEventsRequest{}, fmt.Errorf("--since must be a positive duration, got %s", since)
	}
	if limit <= 0 || limit > cwLogsMaxEventLimit {
		return cwLogsEventsRequest{}, fmt.Errorf("--limit must be between 1 and %d, got %d", cwLogsMaxEventLimit, limit)
	}
	return cwLogsEventsRequest{LogGroup: logGroup, Filter: filter, Since: since, Limit: limit}, nil
}

// cwLogsEventsStart は取得開始時刻 (now から since を引いた時刻) を、FilterLogEvents が受け取る
// RFC 3339 (UTC、秒精度) で返す。
func cwLogsEventsStart(now time.Time, since time.Duration) string {
	return now.Add(-since).UTC().Format(time.RFC3339)
}

// cwLogsFetchEvents は取得条件を FilterLogEvents の引数へ写して 1 ページ分のイベントを返す。
// 対象は 1 ロググループ、終了時刻とページトークンは指定しない (現在時刻までの最新 1 ページ)。
func cwLogsFetchEvents(ctx context.Context, cfg *config.Config, req cwLogsEventsRequest, now time.Time, filterEvents cwLogsFilterEventsFunc) ([]awsinternal.LogEventInfo, error) {
	page, err := filterEvents(ctx, cfg.Profile, cfg.Region, []string{req.LogGroup}, req.Filter, cwLogsEventsStart(now, req.Since), "", "", req.Limit)
	if err != nil {
		return nil, err
	}
	return page.Events, nil
}

func newLogsCmd() *cobra.Command {
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Manage CloudWatch Logs resources",
		Long:  `Provides commands to list CloudWatch Logs log groups and to search log events.`,
	}

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

	// events は 1 ロググループ・期間指定の 1 回取得のみ対応する。複数グループ横断とページング
	// (NextPageToken による続き取得) は gcp logging ls と同じ判断で行わず、Live Tail は
	// WebSocket 前提のため CLI スコープ外とする。
	// <log-group> は名前でも ARN でもよく、そのまま FilterLogEvents の識別子へ渡す。
	eventsCmd := &cobra.Command{
		Use:   "events <log-group>",
		Short: "List log events within a time range (no follow)",
		Long: "Searches events in a log group over the last --since period. " +
			"Events are fetched from the newest and a single page is printed (no pagination, no follow). " +
			"<log-group> is the log group name (or ARN).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := cwLogsEventsRequestFromFlags(cmd, args[0])
			if err != nil {
				return err
			}
			return runList(cmd, ListConfig[awsinternal.LogEventInfo]{
				Columns:  cwLogsEventColumns,
				EmptyMsg: "No log events found",
				Fetch: func(ctx context.Context, cfg *config.Config) ([]awsinternal.LogEventInfo, error) {
					return cwLogsFetchEvents(ctx, cfg, req, time.Now(), awsinternal.FilterLogEvents)
				},
			})
		},
	}
	eventsCmd.Flags().String("filter", "", "CloudWatch Logs filter pattern (e.g. '{ $.level = \"error\" }')")
	eventsCmd.Flags().Duration("since", time.Hour, "How far back to look (e.g. 15m, 1h, 6h, 24h, 168h for 7d)")
	eventsCmd.Flags().Int("limit", cwLogsDefaultEventLimit, "Maximum number of events to fetch (single page; does not paginate; 1 to 10000)")

	logsCmd.AddCommand(lsCmd, eventsCmd)
	return logsCmd
}
