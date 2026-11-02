// Package accesslog は ALB、CloudFront、WAF のアクセスログを共通の形に正規化し、
// アクセス元の IP とリクエスト URI で集計するための型と純粋な処理を提供する。
// AWS SDK には依存しない。
package accesslog

import "time"

// SourceType はアクセスログの種類を表す。
type SourceType string

const (
	// SourceALB は Application Load Balancer のアクセスログを表す。
	SourceALB SourceType = "alb"
	// SourceCloudFront は CloudFront の標準ログを表す。
	SourceCloudFront SourceType = "cloudfront"
	// SourceWAF は AWS WAF のログを表す。
	SourceWAF SourceType = "waf"
)

// Engine は集計に使うクエリエンジンを表す。
type Engine string

const (
	// EngineAthena は Amazon Athena を表す。
	EngineAthena Engine = "athena"
	// EngineLogsInsights は CloudWatch Logs Insights を表す。
	EngineLogsInsights Engine = "logs_insights"
)

// DestinationType はログの出力先の種類を表す。
type DestinationType string

const (
	// DestinationNone はログの設定が無いことを表す。
	DestinationNone DestinationType = "none"
	// DestinationS3 は Amazon S3 を表す。
	DestinationS3 DestinationType = "s3"
	// DestinationCloudWatchLogs は CloudWatch Logs を表す。
	DestinationCloudWatchLogs DestinationType = "cloudwatch_logs"
	// DestinationFirehose は Amazon Data Firehose を表す。
	DestinationFirehose DestinationType = "firehose"
)

// SchemaProfile は Athena のテーブルの列の並びを表す。
type SchemaProfile string

const (
	// SchemaALBLegacy は ALB の legacy アクセスログのプロファイルを表す。
	SchemaALBLegacy SchemaProfile = "alb_legacy"
	// SchemaCloudFrontLegacy は CloudFront の legacy 標準ログのプロファイルを表す。
	SchemaCloudFrontLegacy SchemaProfile = "cloudfront_legacy"
	// SchemaCloudFrontV2JSON は CloudFront の標準ログ v2 の JSON のプロファイルを表す。
	SchemaCloudFrontV2JSON SchemaProfile = "cloudfront_v2_json"
	// SchemaWAFJSON は WAF の S3 ログの JSON のプロファイルを表す。
	SchemaWAFJSON SchemaProfile = "waf_json"
)

// PartitionStyle は Athena のテーブルのパーティションの形式を表す。
type PartitionStyle string

const (
	// PartitionNone はパーティションが無いことを表す。
	PartitionNone PartitionStyle = ""
	// PartitionDay は ALB の day (yyyy/MM/dd) のパーティションを表す。
	PartitionDay PartitionStyle = "day"
	// PartitionYearMonthDayHour は CloudFront と WAF の year/month/day/hour のパーティションを表す。
	PartitionYearMonthDayHour PartitionStyle = "year_month_day_hour"
	// PartitionLogTime は WAF の log_time (yyyy/MM/dd/HH/mm) のパーティションを表す。
	PartitionLogTime PartitionStyle = "log_time"
)

// Destination はログの出力先を表す。
type Destination struct {
	Type           DestinationType `json:"type"`
	Bucket         string          `json:"bucket,omitempty"`
	Prefix         string          `json:"prefix,omitempty"`
	LogGroup       string          `json:"log_group,omitempty"`
	Format         string          `json:"format,omitempty"`
	FieldDelimiter string          `json:"field_delimiter,omitempty"`
	RecordFields   []string        `json:"record_fields,omitempty"`
	SuffixPath     string          `json:"suffix_path,omitempty"`
	HiveCompatible bool            `json:"hive_compatible,omitempty"`
}

// SourceStatus はソースの状態を表す。状態は 1 つの値にまとめず、
// ログの有効、出力先の種類、読み取り可否、テーブルの有効、データの有無を独立して持つ。
type SourceStatus struct {
	LoggingEnabled      bool            `json:"logging_enabled"`
	DestinationType     DestinationType `json:"destination_type"`
	DestinationReadable bool            `json:"destination_readable"`
	TableConfigured     bool            `json:"table_configured"`
	TableValid          bool            `json:"table_valid"`
	DataPresent         *bool           `json:"data_present,omitempty"`
	LastDeliveryAt      string          `json:"last_delivery_at,omitempty"`
	SourceKey           string          `json:"source_key,omitempty"`
	Attribution         bool            `json:"attribution"`
	SharedBy            []string        `json:"shared_by,omitempty"`
	Issues              []string        `json:"issues,omitempty"`
}

// Source は集計の対象になるログのソースを表す。
type Source struct {
	ID              string        `json:"id"`
	Type            SourceType    `json:"type"`
	Name            string        `json:"name"`
	Region          string        `json:"region"`
	QueryRegion     string        `json:"query_region"`
	Global          bool          `json:"global,omitempty"`
	AccountID       string        `json:"account_id,omitempty"`
	ResourceARN     string        `json:"resource_arn,omitempty"`
	ResourceID      string        `json:"resource_id,omitempty"`
	DNSName         string        `json:"dns_name,omitempty"`
	DomainName      string        `json:"domain_name,omitempty"`
	Aliases         []string      `json:"aliases,omitempty"`
	OriginOf        string        `json:"origin_of,omitempty"`
	Engine          Engine        `json:"engine"`
	Destination     Destination   `json:"destination"`
	SchemaProfile   SchemaProfile `json:"schema_profile,omitempty"`
	Status          SourceStatus  `json:"status"`
	SourceKeyValues []string      `json:"source_key_values,omitempty"`
	SuggestedTable  string        `json:"suggested_table,omitempty"`
	SetupSQL        string        `json:"setup_sql,omitempty"`
}

// TableMapping はソースに対応付ける Athena のテーブルを表す。
type TableMapping struct {
	Catalog         string         `json:"catalog"`
	Database        string         `json:"database"`
	Table           string         `json:"table"`
	SchemaProfile   SchemaProfile  `json:"schema_profile"`
	PartitionStyle  PartitionStyle `json:"partition_style,omitempty"`
	PartitionKeys   []string       `json:"partition_keys,omitempty"`
	SourceKeyColumn string         `json:"source_key_column,omitempty"`
}

// TableValidation は Athena のテーブルの検証結果を表す。
type TableValidation struct {
	Valid           bool          `json:"valid"`
	SchemaProfile   SchemaProfile `json:"schema_profile,omitempty"`
	Columns         []string      `json:"columns,omitempty"`
	PartitionKeys   []string      `json:"partition_keys,omitempty"`
	MissingColumns  []string      `json:"missing_columns,omitempty"`
	SourceKeyColumn string        `json:"source_key_column,omitempty"`
	Attribution     bool          `json:"attribution"`
	Location        string        `json:"location,omitempty"`
	InputFormat     string        `json:"input_format,omitempty"`
	SerDe           string        `json:"serde,omitempty"`
	Issues          []string      `json:"issues,omitempty"`
}

// QueryState は集計クエリの状態を表す。
type QueryState string

const (
	// QueryStateQueued はクエリが実行待ちであることを表す。
	QueryStateQueued QueryState = "queued"
	// QueryStateRunning はクエリが実行中であることを表す。
	QueryStateRunning QueryState = "running"
	// QueryStateSucceeded はクエリが成功したことを表す。
	QueryStateSucceeded QueryState = "succeeded"
	// QueryStatePartial は一部のソースだけが成功したことを表す。
	QueryStatePartial QueryState = "partial"
	// QueryStateFailed はクエリが失敗したことを表す。
	QueryStateFailed QueryState = "failed"
	// QueryStateCancelled はクエリが取り消されたことを表す。
	QueryStateCancelled QueryState = "cancelled"
)

// TimeRange は集計の対象期間を表す。時刻は UTC で扱う。
type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Validate は期間の前後関係を検証する。
func (r TimeRange) Validate() error {
	if r.From.IsZero() || r.To.IsZero() {
		return ErrInvalidTimeRange
	}
	if !r.From.Before(r.To) {
		return ErrInvalidTimeRange
	}
	return nil
}

// Filter は集計の絞り込み条件を表す。
type Filter struct {
	OnlyBlocked   bool     `json:"only_blocked,omitempty"`
	StatusClasses []string `json:"status_classes,omitempty"`
	IP            string   `json:"ip,omitempty"`
	URIPrefix     string   `json:"uri_prefix,omitempty"`
}

// PairRow はアクセス元の IP と URI の組の集計結果を表す。
type PairRow struct {
	SourceID         string           `json:"-"`
	SourceKey        string           `json:"source_key,omitempty"`
	ClientIP         string           `json:"client_ip"`
	URIPath          string           `json:"uri_path"`
	RequestCount     int64            `json:"request_count"`
	BlockedCount     int64            `json:"blocked_count"`
	ChallengedCount  int64            `json:"challenged_count"`
	CountMatched     int64            `json:"count_matched"`
	ClientErrorCount int64            `json:"client_error_count"`
	ServerErrorCount int64            `json:"server_error_count"`
	Sources          []string         `json:"sources,omitempty"`
	SourceCounts     map[string]int64 `json:"source_counts,omitempty"`
	FirstSeen        *time.Time       `json:"first_seen,omitempty"`
	LastSeen         *time.Time       `json:"last_seen,omitempty"`
	SampleUserAgent  string           `json:"sample_user_agent,omitempty"`
	SampleRuleID     string           `json:"sample_rule_id,omitempty"`
	HasQuery         bool             `json:"has_query,omitempty"`
	SampleQuery      string           `json:"sample_query,omitempty"`
	Flags            []string         `json:"flags,omitempty"`
}

// IPSummary はアクセス元の IP 単独の集計結果を表す。
type IPSummary struct {
	SourceID         string           `json:"-"`
	SourceKey        string           `json:"source_key,omitempty"`
	ClientIP         string           `json:"client_ip"`
	RequestCount     int64            `json:"request_count"`
	BlockedCount     int64            `json:"blocked_count"`
	ChallengedCount  int64            `json:"challenged_count"`
	CountMatched     int64            `json:"count_matched"`
	ClientErrorCount int64            `json:"client_error_count"`
	ServerErrorCount int64            `json:"server_error_count"`
	DistinctURIs     int64            `json:"distinct_uris"`
	Sources          []string         `json:"sources,omitempty"`
	SourceCounts     map[string]int64 `json:"source_counts,omitempty"`
	FirstSeen        *time.Time       `json:"first_seen,omitempty"`
	LastSeen         *time.Time       `json:"last_seen,omitempty"`
	Flags            []string         `json:"flags,omitempty"`
}

// SourceSummary はソースごとの集計の統計を表す。
type SourceSummary struct {
	SourceID       string `json:"source_id"`
	Records        int64  `json:"records"`
	Blocked        int64  `json:"blocked"`
	Challenged     int64  `json:"challenged"`
	ClientErrors   int64  `json:"client_errors"`
	ServerErrors   int64  `json:"server_errors"`
	BytesScanned   int64  `json:"bytes_scanned"`
	RecordsScanned int64  `json:"records_scanned"`
}

// Summary は集計全体のサマリを表す。
type Summary struct {
	SourceCount      int             `json:"source_count"`
	RecordCount      int64           `json:"record_count"`
	BlockedCount     int64           `json:"blocked_count"`
	ChallengedCount  int64           `json:"challenged_count"`
	ClientErrorCount int64           `json:"client_error_count"`
	ServerErrorCount int64           `json:"server_error_count"`
	DistinctIPs      int             `json:"distinct_ips"`
	DistinctURIs     int             `json:"distinct_uris"`
	BySources        []SourceSummary `json:"by_sources,omitempty"`
	DuplicateWarning bool            `json:"duplicate_warning"`
}

// SourceResult は 1 つのソースの取得結果を表す。
type SourceResult struct {
	SourceID       string
	Rows           []PairRow
	IPs            []IPSummary
	BytesScanned   int64
	RecordsScanned int64
	Err            error
}

// Result は集計全体の結果を表す。
type Result struct {
	Rows     []PairRow   `json:"rows"`
	IPs      []IPSummary `json:"ips,omitempty"`
	Summary  Summary     `json:"summary"`
	Warnings []string    `json:"warnings,omitempty"`
}
