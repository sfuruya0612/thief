package api

import (
	"context"
	"errors"
	"time"

	awsinternal "github.com/sfuruya0612/thief/backend/internal/aws"
)

// accessLogDeps は Access Logs のハンドラが使う外部依存である。
// テストで実 AWS へ接続せずに差し替えられるよう、関数として持つ。
type accessLogDeps struct {
	discover      func(ctx context.Context, profile, region string) (awsinternal.AccessLogSources, error)
	tableMetadata func(ctx context.Context, profile, region, catalog, database, table, workgroup string) (*awsinternal.AthenaTableMetadata, error)
	workgroup     func(ctx context.Context, profile, region, name string) (*awsinternal.AthenaWorkgroupConfig, error)
	startAthena   func(ctx context.Context, profile, region string, in awsinternal.StartAthenaQueryInput) (*awsinternal.AthenaQueryExecution, error)
	getAthena     func(ctx context.Context, profile, region, id string) (*awsinternal.AthenaQueryExecution, error)
	resultsAthena func(ctx context.Context, profile, region, id string, max int) (*awsinternal.AthenaResultPage, error)
	stopAthena    func(ctx context.Context, profile, region, id string) error
	startInsights func(ctx context.Context, profile, region string, logGroups []string, query string, start, end time.Time, limit int) (string, error)
	getInsights   func(ctx context.Context, profile, region, id string) (*awsinternal.LogsInsightsQueryResult, error)
	stopInsights  func(ctx context.Context, profile, region, id string) error
}

func defaultAccessLogDeps() *accessLogDeps {
	return &accessLogDeps{
		discover:      awsinternal.DiscoverAccessLogSources,
		tableMetadata: awsinternal.GetAthenaTableMetadata,
		workgroup:     awsinternal.GetAthenaWorkgroupConfig,
		startAthena:   awsinternal.StartAthenaQuery,
		getAthena:     awsinternal.GetAthenaQuery,
		resultsAthena: awsinternal.GetAthenaQueryResultsAll,
		stopAthena:    awsinternal.StopAthenaQuery,
		startInsights: awsinternal.StartLogsInsightsQuery,
		getInsights:   awsinternal.GetLogsInsightsQueryResults,
		stopInsights:  awsinternal.StopLogsInsightsQuery,
	}
}

// unconfiguredAccessLogDeps はテストで実 AWS へ接続しないための既定値である。
func unconfiguredAccessLogDeps() *accessLogDeps {
	err := errors.New("access log deps are not configured in newTestServer")
	return &accessLogDeps{
		discover: func(context.Context, string, string) (awsinternal.AccessLogSources, error) {
			return awsinternal.AccessLogSources{}, err
		},
		tableMetadata: func(context.Context, string, string, string, string, string, string) (*awsinternal.AthenaTableMetadata, error) {
			return nil, err
		},
		workgroup: func(context.Context, string, string, string) (*awsinternal.AthenaWorkgroupConfig, error) {
			return nil, err
		},
		startAthena: func(context.Context, string, string, awsinternal.StartAthenaQueryInput) (*awsinternal.AthenaQueryExecution, error) {
			return nil, err
		},
		getAthena: func(context.Context, string, string, string) (*awsinternal.AthenaQueryExecution, error) {
			return nil, err
		},
		resultsAthena: func(context.Context, string, string, string, int) (*awsinternal.AthenaResultPage, error) {
			return nil, err
		},
		stopAthena: func(context.Context, string, string, string) error { return err },
		startInsights: func(context.Context, string, string, []string, string, time.Time, time.Time, int) (string, error) {
			return "", err
		},
		getInsights: func(context.Context, string, string, string) (*awsinternal.LogsInsightsQueryResult, error) {
			return nil, err
		},
		stopInsights: func(context.Context, string, string, string) error { return err },
	}
}
