package accesslog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sfuruya0612/thief/backend/internal/sqlguard"
)

const (
	// DefaultLimit は集計結果の既定の行数である。
	DefaultLimit = 200
	// MaxLimit は集計結果の上限の行数である。
	MaxLimit = 200
)

var identRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidateIdent は Athena のデータベース名、テーブル名、カタログ名に使える
// 識別子かを返す。
func ValidateIdent(s string) bool {
	return identRe.MatchString(s)
}

// AthenaSource は Athena のクエリに含めるソースを表す。
type AthenaSource struct {
	ID        string
	KeyValues []string
}

// AthenaPlan は Athena の集計クエリを組み立てるための入力を表す。
type AthenaPlan struct {
	Database        string
	Table           string
	Profile         SchemaProfile
	Columns         []string
	PartitionStyle  PartitionStyle
	PartitionKeys   []string
	SourceKeyColumn string
	Sources         []AthenaSource
	TimeRange       TimeRange
	Filter          Filter
	Limit           int
}

// AthenaQuery は生成した Athena のクエリを表す。
type AthenaQuery struct {
	SQL           string
	Parameters    []string
	SourceKeyToID map[string]string
	FullScan      bool
}

// BuildPairQuery はアクセス元の IP と URI の組の集計クエリを生成する。
func BuildPairQuery(plan AthenaPlan) (AthenaQuery, error) {
	return buildAthenaQuery(plan, false)
}

// BuildIPQuery はアクセス元の IP 単独の集計クエリを生成する。
func BuildIPQuery(plan AthenaPlan) (AthenaQuery, error) {
	return buildAthenaQuery(plan, true)
}

func buildAthenaQuery(plan AthenaPlan, ipSummary bool) (AthenaQuery, error) {
	if err := validatePlan(plan); err != nil {
		return AthenaQuery{}, err
	}
	profile, err := ProfileForColumns(plan.Profile, plan.Columns)
	if err != nil {
		return AthenaQuery{}, err
	}
	limit := plan.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	params := make([]string, 0, 16)
	where := make([]string, 0, 8)

	partitionSQL, partitionParams, fullScan := partitionPredicates(plan)
	if partitionSQL != "" {
		where = append(where, partitionSQL)
		params = append(params, partitionParams...)
	}

	where = append(where, fmt.Sprintf("%s >= CAST(? AS BIGINT)", profile.TimestampExpr))
	params = append(params, strconv.FormatInt(plan.TimeRange.From.UnixMilli(), 10))
	where = append(where, fmt.Sprintf("%s < CAST(? AS BIGINT)", profile.TimestampExpr))
	params = append(params, strconv.FormatInt(plan.TimeRange.To.UnixMilli(), 10))

	sourceExpr := "''"
	if plan.SourceKeyColumn != "" {
		sourceExpr = "CAST(" + quoteIdent(plan.SourceKeyColumn) + " AS varchar)"
		sourceValues := make([]string, 0, len(plan.Sources))
		for _, source := range plan.Sources {
			sourceValues = append(sourceValues, source.KeyValues...)
		}
		if len(sourceValues) > 0 {
			placeholders := make([]string, len(sourceValues))
			for i := range sourceValues {
				placeholders[i] = "?"
			}
			where = append(where, fmt.Sprintf("%s IN (%s)", sourceExpr, strings.Join(placeholders, ", ")))
			params = append(params, sourceValues...)
		}
	}

	if plan.Filter.IP != "" {
		where = append(where, fmt.Sprintf("%s = ?", profile.ClientIPExpr))
		params = append(params, plan.Filter.IP)
	}
	if plan.Filter.URIPrefix != "" {
		where = append(where, fmt.Sprintf("strpos(%s, ?) = 1", profile.URIPathExpr))
		params = append(params, plan.Filter.URIPrefix)
	}
	if plan.Filter.OnlyBlocked {
		where = append(where, "("+profile.BlockedExpr+")")
	}
	if len(plan.Filter.StatusClasses) > 0 {
		classes := statusClassPredicates(profile.StatusExpr, plan.Filter.StatusClasses)
		if len(classes) > 0 {
			where = append(where, "("+strings.Join(classes, " OR ")+")")
		}
	}

	base := fmt.Sprintf(`WITH base AS (
  SELECT
    %s AS source_key,
    %s AS client_ip,
    %s AS uri_path,
    %s AS uri_query,
    %s AS status_code,
    %s AS method,
    %s AS action,
    %s AS user_agent,
    %s AS rule_id,
    %s AS ts_ms,
    %s AS blocked,
    %s AS challenged,
    %s AS count_matched
  FROM %s.%s
  WHERE %s
)`,
		sourceExpr,
		profile.ClientIPExpr,
		profile.URIPathExpr,
		profile.URIQueryExpr,
		profile.StatusExpr,
		profile.MethodExpr,
		profile.ActionExpr,
		profile.UserAgentExpr,
		profile.RuleIDExpr,
		profile.TimestampExpr,
		profile.BlockedExpr,
		profile.ChallengedExpr,
		profile.CountMatchedExpr,
		quoteIdent(plan.Database),
		quoteIdent(plan.Table),
		strings.Join(where, "\n    AND "),
	)

	var body string
	if ipSummary {
		body = `SELECT
  source_key,
  client_ip,
  count(*) AS request_count,
  sum(CASE WHEN blocked THEN 1 ELSE 0 END) AS blocked_count,
  sum(CASE WHEN challenged THEN 1 ELSE 0 END) AS challenged_count,
  sum(CASE WHEN count_matched THEN 1 ELSE 0 END) AS count_matched,
  sum(CASE WHEN status_code BETWEEN 400 AND 499 THEN 1 ELSE 0 END) AS client_error_count,
  sum(CASE WHEN status_code >= 500 THEN 1 ELSE 0 END) AS server_error_count,
  count(DISTINCT uri_path) AS distinct_uris,
  min(ts_ms) AS first_seen_ms,
  max(ts_ms) AS last_seen_ms
FROM base
GROUP BY source_key, client_ip
ORDER BY blocked_count DESC, challenged_count DESC, client_error_count DESC, request_count DESC, last_seen_ms DESC, client_ip ASC
LIMIT ` + strconv.Itoa(limit)
	} else {
		body = `SELECT
  source_key,
  client_ip,
  uri_path,
  count(*) AS request_count,
  sum(CASE WHEN blocked THEN 1 ELSE 0 END) AS blocked_count,
  sum(CASE WHEN challenged THEN 1 ELSE 0 END) AS challenged_count,
  sum(CASE WHEN count_matched THEN 1 ELSE 0 END) AS count_matched,
  sum(CASE WHEN status_code BETWEEN 400 AND 499 THEN 1 ELSE 0 END) AS client_error_count,
  sum(CASE WHEN status_code >= 500 THEN 1 ELSE 0 END) AS server_error_count,
  min(ts_ms) AS first_seen_ms,
  max(ts_ms) AS last_seen_ms,
  max_by(user_agent, ts_ms) AS sample_user_agent,
  max_by(rule_id, ts_ms) AS sample_rule_id,
  max(CASE WHEN uri_query IS NOT NULL AND uri_query <> '' AND uri_query <> '-' THEN 1 ELSE 0 END) AS has_query,
  max_by(uri_query, ts_ms) AS sample_query
FROM base
GROUP BY source_key, client_ip, uri_path
ORDER BY blocked_count DESC, challenged_count DESC, client_error_count DESC, request_count DESC, last_seen_ms DESC, client_ip ASC, uri_path ASC
LIMIT ` + strconv.Itoa(limit)
	}

	sql := base + "\n" + body
	if err := sqlguard.ValidateReadOnly(sql); err != nil {
		return AthenaQuery{}, fmt.Errorf("validate generated athena query: %w", err)
	}

	return AthenaQuery{
		SQL:           sql,
		Parameters:    params,
		SourceKeyToID: sourceKeyToID(plan.Sources),
		FullScan:      fullScan,
	}, nil
}

// InsightsPlan は CloudWatch Logs Insights のクエリを組み立てるための入力を表す。
type InsightsPlan struct {
	LogGroups []string
	Sources   []AthenaSource
	TimeRange TimeRange
	Filter    Filter
	Limit     int
}

// InsightsQuery は生成した Logs Insights のクエリを表す。
type InsightsQuery struct {
	Query         string
	SourceKeyToID map[string]string
}

// BuildWAFInsightsQuery は WAF の CloudWatch Logs の集計クエリを生成する。
func BuildWAFInsightsQuery(plan InsightsPlan) (InsightsQuery, error) {
	if len(plan.LogGroups) == 0 {
		return InsightsQuery{}, fmt.Errorf("%w: no log group", ErrInvalidSource)
	}
	if err := plan.TimeRange.Validate(); err != nil {
		return InsightsQuery{}, err
	}
	limit := plan.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	var filters []string
	if sourceFilter := insightsSourceFilter(plan.Sources); sourceFilter != "" {
		filters = append(filters, sourceFilter)
	}
	if plan.Filter.IP != "" {
		filters = append(filters, fmt.Sprintf(`httpRequest.clientIp = "%s"`, escapeInsights(plan.Filter.IP)))
	}
	if plan.Filter.URIPrefix != "" {
		filters = append(filters, fmt.Sprintf(`startsWith(httpRequest.uri, "%s")`, escapeInsights(plan.Filter.URIPrefix)))
	}
	if plan.Filter.OnlyBlocked {
		filters = append(filters, `action = "BLOCK"`)
	}
	query := "fields httpRequest.clientIp as ip, httpRequest.uri as uri, action, webaclId, timestamp"
	if len(filters) > 0 {
		query += " | filter " + strings.Join(filters, " and ")
	}
	query += ` | stats count(*) as request_count, min(timestamp) as first_seen_ms, max(timestamp) as last_seen_ms by ip, uri, action, webaclId
| sort request_count desc
| limit ` + strconv.Itoa(limit)
	return InsightsQuery{
		Query:         query,
		SourceKeyToID: sourceKeyToID(plan.Sources),
	}, nil
}

func validatePlan(plan AthenaPlan) error {
	if plan.Database == "" || plan.Table == "" {
		return fmt.Errorf("%w: database and table are required", ErrInvalidTable)
	}
	if !identRe.MatchString(plan.Database) {
		return fmt.Errorf("%w: database %q", ErrInvalidTable, plan.Database)
	}
	if !identRe.MatchString(plan.Table) {
		return fmt.Errorf("%w: table %q", ErrInvalidTable, plan.Table)
	}
	if err := plan.TimeRange.Validate(); err != nil {
		return err
	}
	if _, err := ProfileFor(plan.Profile); err != nil {
		return err
	}
	if plan.SourceKeyColumn != "" {
		for _, source := range plan.Sources {
			if len(source.KeyValues) == 0 {
				return fmt.Errorf("%w: source %s has no key value", ErrAttributionRequired, source.ID)
			}
		}
	}
	return nil
}

func partitionPredicates(plan AthenaPlan) (string, []string, bool) {
	if len(plan.PartitionKeys) == 0 || plan.PartitionStyle == PartitionNone {
		return "", nil, true
	}
	keys := make(map[string]string, len(plan.PartitionKeys))
	for _, key := range plan.PartitionKeys {
		keys[strings.ToLower(key)] = key
	}
	from := plan.TimeRange.From.AddDate(0, 0, -1).UTC()
	to := plan.TimeRange.To.AddDate(0, 0, 1).UTC()

	switch plan.PartitionStyle {
	case PartitionDay:
		key, ok := keys["day"]
		if !ok {
			return "", nil, true
		}
		return fmt.Sprintf("%s BETWEEN ? AND ?", quoteIdent(key)),
			[]string{from.Format("2006/01/02"), to.Format("2006/01/02")}, false
	case PartitionLogTime:
		key, ok := keys["log_time"]
		if !ok {
			return "", nil, true
		}
		return fmt.Sprintf("%s BETWEEN ? AND ?", quoteIdent(key)),
			[]string{from.Add(-time.Hour).Format("2006/01/02/15/04"), to.Add(time.Hour).Format("2006/01/02/15/04")}, false
	case PartitionYearMonthDayHour:
		yearKey, okY := keys["year"]
		monthKey, okM := keys["month"]
		dayKey, okD := keys["day"]
		if !okY || !okM || !okD {
			return "", nil, true
		}
		var groups []string
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			groups = append(groups, fmt.Sprintf("(%s = %d AND %s = %d AND %s = %d)",
				quoteIdent(yearKey), d.Year(),
				quoteIdent(monthKey), int(d.Month()),
				quoteIdent(dayKey), d.Day()))
		}
		if len(groups) == 0 {
			return "", nil, true
		}
		return "(" + strings.Join(groups, " OR ") + ")", nil, false
	default:
		return "", nil, true
	}
}

func statusClassPredicates(expr string, classes []string) []string {
	seen := make(map[string]bool, len(classes))
	var predicates []string
	for _, class := range classes {
		class = strings.ToLower(strings.TrimSpace(class))
		if seen[class] {
			continue
		}
		seen[class] = true
		switch class {
		case "4xx":
			predicates = append(predicates, fmt.Sprintf("%s BETWEEN 400 AND 499", expr))
		case "5xx":
			predicates = append(predicates, fmt.Sprintf("%s >= 500", expr))
		}
	}
	return predicates
}

func sourceKeyToID(sources []AthenaSource) map[string]string {
	m := make(map[string]string, len(sources))
	for _, source := range sources {
		for _, key := range source.KeyValues {
			m[key] = source.ID
		}
	}
	return m
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func escapeInsights(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

func insightsSourceFilter(sources []AthenaSource) string {
	var values []string
	seen := make(map[string]struct{})
	for _, source := range sources {
		for _, value := range source.KeyValues {
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf(`webaclId = "%s"`, escapeInsights(value)))
	}
	return "(" + strings.Join(parts, " or ") + ")"
}
