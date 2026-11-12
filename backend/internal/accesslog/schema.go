package accesslog

import (
	"fmt"
	"slices"
	"strings"
)

// Profile は Athena のテーブルの列から集計に使う式を組み立てるための定義を表す。
type Profile struct {
	Name             SchemaProfile
	SourceType       SourceType
	RequiredColumns  []string
	SourceKeyColumns []string
	ClientIPExpr     string
	URIPathExpr      string
	URIQueryExpr     string
	StatusExpr       string
	MethodExpr       string
	ActionExpr       string
	RuleIDExpr       string
	UserAgentExpr    string
	TimestampExpr    string
	BlockedExpr      string
	ChallengedExpr   string
	CountMatchedExpr string
}

// ErrUnknownProfile はスキーマプロファイルが定義されていないことを表す。
var ErrUnknownProfile = fmt.Errorf("%w: unknown schema profile", ErrUnsupportedProfile)

var profiles = map[SchemaProfile]Profile{
	SchemaALBLegacy: {
		Name:             SchemaALBLegacy,
		SourceType:       SourceALB,
		RequiredColumns:  []string{"elb", "client_ip", "request_url", "elb_status_code", "time", "actions_executed", "user_agent"},
		SourceKeyColumns: []string{"elb"},
		ClientIPExpr:     "client_ip",
		URIPathExpr:      "regexp_extract(request_url, '^(?:[a-zA-Z][a-zA-Z0-9+.-]*://[^/]*)?([^?#]*)', 1)",
		URIQueryExpr:     "regexp_extract(request_url, '\\?([^#]*)', 1)",
		StatusExpr:       "elb_status_code",
		MethodExpr:       "regexp_extract(request_url, '^([A-Z]+) ', 1)",
		ActionExpr:       "''",
		RuleIDExpr:       "''",
		UserAgentExpr:    "user_agent",
		TimestampExpr:    "CAST(to_unixtime(from_iso8601_timestamp(time)) * 1000 AS BIGINT)",
		BlockedExpr:      "regexp_like(actions_executed, '(^|,)waf($|,)')",
		ChallengedExpr:   "false",
		CountMatchedExpr: "false",
	},
	SchemaCloudFrontLegacy: {
		Name:             SchemaCloudFrontLegacy,
		SourceType:       SourceCloudFront,
		RequiredColumns:  []string{"c_ip", "cs_uri_stem", "sc_status", "date", "time", "cs_user_agent"},
		SourceKeyColumns: []string{"cs_host", "x_host_header"},
		ClientIPExpr:     "c_ip",
		URIPathExpr:      "cs_uri_stem",
		URIQueryExpr:     "cs_uri_query",
		StatusExpr:       "sc_status",
		MethodExpr:       "cs_method",
		ActionExpr:       "''",
		RuleIDExpr:       "''",
		UserAgentExpr:    "cs_user_agent",
		TimestampExpr:    "CAST(to_unixtime(from_iso8601_timestamp(concat(CAST(\"date\" AS varchar), 'T', time, 'Z'))) * 1000 AS BIGINT)",
		BlockedExpr:      "false",
		ChallengedExpr:   "false",
		CountMatchedExpr: "false",
	},
	SchemaCloudFrontV2JSON: {
		Name:             SchemaCloudFrontV2JSON,
		SourceType:       SourceCloudFront,
		RequiredColumns:  []string{"c-ip", "cs-uri-stem", "sc-status", "date", "time", "cs(User-Agent)"},
		SourceKeyColumns: []string{"distributionid", "cs(Host)", "x-host-header"},
		ClientIPExpr:     "\"c-ip\"",
		URIPathExpr:      "\"cs-uri-stem\"",
		URIQueryExpr:     "\"cs-uri-query\"",
		StatusExpr:       "CAST(\"sc-status\" AS int)",
		MethodExpr:       "\"cs-method\"",
		ActionExpr:       "''",
		RuleIDExpr:       "''",
		UserAgentExpr:    "\"cs(User-Agent)\"",
		TimestampExpr:    "CAST(to_unixtime(from_iso8601_timestamp(concat(CAST(\"date\" AS varchar), 'T', time, 'Z'))) * 1000 AS BIGINT)",
		BlockedExpr:      "false",
		ChallengedExpr:   "false",
		CountMatchedExpr: "false",
	},
	SchemaWAFJSON: {
		Name:             SchemaWAFJSON,
		SourceType:       SourceWAF,
		RequiredColumns:  []string{"httprequest", "action", "timestamp", "webaclid", "terminatingruleid"},
		SourceKeyColumns: []string{"webaclid"},
		ClientIPExpr:     "httprequest.clientip",
		URIPathExpr:      "httprequest.uri",
		URIQueryExpr:     "httprequest.args",
		StatusExpr:       "0",
		MethodExpr:       "httprequest.httpmethod",
		ActionExpr:       "upper(action)",
		RuleIDExpr:       "terminatingruleid",
		UserAgentExpr:    "''",
		TimestampExpr:    "timestamp",
		BlockedExpr:      "upper(action) = 'BLOCK'",
		ChallengedExpr:   "upper(action) IN ('CAPTCHA', 'CHALLENGE')",
		CountMatchedExpr: "coalesce(cardinality(nonterminatingmatchingrules), 0) > 0 OR coalesce(cardinality(rulegrouplist), 0) > 0",
	},
}

// ProfileFor はスキーマプロファイルの定義を返す。
func ProfileFor(name SchemaProfile) (Profile, error) {
	p, ok := profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("%w: %s", ErrUnknownProfile, name)
	}
	return p, nil
}

// ProfileForColumns はテーブルの実際の列名を使ってスキーマプロファイルの式を組み立てる。
// 列名の大文字と小文字や、CloudFront の標準ログ v2 の記号を含む列名の差を吸収する。
func ProfileForColumns(name SchemaProfile, columns []string) (Profile, error) {
	p, err := ProfileFor(name)
	if err != nil {
		return p, err
	}
	col := func(name string) string {
		if actual := findColumn(columns, name); actual != "" {
			return quoteIdent(actual)
		}
		return quoteIdent(name)
	}
	switch name {
	case SchemaALBLegacy:
		requestURL := col("request_url")
		p.ClientIPExpr = col("client_ip")
		p.URIPathExpr = fmt.Sprintf("regexp_extract(%s, '^(?:[a-zA-Z][a-zA-Z0-9+.-]*://[^/]*)?([^?#]*)', 1)", requestURL)
		p.URIQueryExpr = fmt.Sprintf("regexp_extract(%s, '\\?([^#]*)', 1)", requestURL)
		p.StatusExpr = col("elb_status_code")
		p.MethodExpr = fmt.Sprintf("regexp_extract(%s, '^([A-Z]+) ', 1)", requestURL)
		p.UserAgentExpr = col("user_agent")
		p.TimestampExpr = fmt.Sprintf("CAST(to_unixtime(from_iso8601_timestamp(%s)) * 1000 AS BIGINT)", col("time"))
		p.BlockedExpr = fmt.Sprintf("regexp_like(%s, '(^|,)waf($|,)')", col("actions_executed"))
	case SchemaCloudFrontLegacy:
		p.ClientIPExpr = col("c_ip")
		p.URIPathExpr = col("cs_uri_stem")
		p.URIQueryExpr = col("cs_uri_query")
		p.StatusExpr = col("sc_status")
		p.MethodExpr = col("cs_method")
		p.UserAgentExpr = col("cs_user_agent")
		p.TimestampExpr = fmt.Sprintf("CAST(to_unixtime(from_iso8601_timestamp(concat(CAST(%s AS varchar), 'T', %s, 'Z'))) * 1000 AS BIGINT)", col("date"), col("time"))
	case SchemaCloudFrontV2JSON:
		p.ClientIPExpr = col("c-ip")
		p.URIPathExpr = col("cs-uri-stem")
		p.URIQueryExpr = col("cs-uri-query")
		p.StatusExpr = fmt.Sprintf("CAST(%s AS int)", col("sc-status"))
		p.MethodExpr = col("cs-method")
		p.UserAgentExpr = col("cs(User-Agent)")
		p.TimestampExpr = fmt.Sprintf("CAST(to_unixtime(from_iso8601_timestamp(concat(CAST(%s AS varchar), 'T', %s, 'Z'))) * 1000 AS BIGINT)", col("date"), col("time"))
	}
	return p, nil
}

// DetectProfile はテーブルの列名からスキーマプロファイルを判定する。
// 複数のプロファイルに一致する場合は、最初に一致したものを返す。
func DetectProfile(columns []string) (SchemaProfile, bool) {
	lower := make([]string, 0, len(columns))
	for _, c := range columns {
		lower = append(lower, strings.ToLower(c))
	}
	order := []SchemaProfile{SchemaALBLegacy, SchemaCloudFrontLegacy, SchemaCloudFrontV2JSON, SchemaWAFJSON}
	for _, name := range order {
		p := profiles[name]
		if containsAll(lower, p.RequiredColumns) {
			return name, true
		}
	}
	return "", false
}

// MissingColumns は必須列のうち、テーブルに無いものを返す。
func MissingColumns(name SchemaProfile, columns []string) []string {
	p, err := ProfileFor(name)
	if err != nil {
		return nil
	}
	lower := make(map[string]struct{}, len(columns))
	for _, c := range columns {
		lower[strings.ToLower(c)] = struct{}{}
	}
	var missing []string
	for _, c := range p.RequiredColumns {
		if _, ok := lower[strings.ToLower(c)]; !ok {
			missing = append(missing, c)
		}
	}
	return missing
}

// FindSourceKeyColumn はテーブルの列からソースの識別子に使う列を返す。
// 見つからない場合は空文字列を返す。
func FindSourceKeyColumn(name SchemaProfile, columns []string) string {
	p, err := ProfileFor(name)
	if err != nil {
		return ""
	}
	for _, candidate := range p.SourceKeyColumns {
		if actual := findColumn(columns, candidate); actual != "" {
			return actual
		}
	}
	return ""
}

// DetectPartitionStyle はテーブルのパーティションキーからパーティションの形式を判定する。
func DetectPartitionStyle(partitionKeys []string) PartitionStyle {
	lower := make([]string, 0, len(partitionKeys))
	for _, k := range partitionKeys {
		lower = append(lower, strings.ToLower(k))
	}
	switch {
	case slices.Contains(lower, "log_time"):
		return PartitionLogTime
	case slices.Contains(lower, "year") && slices.Contains(lower, "month") && slices.Contains(lower, "day"):
		return PartitionYearMonthDayHour
	case slices.Contains(lower, "day"):
		return PartitionDay
	default:
		return PartitionNone
	}
}

func findColumn(columns []string, name string) string {
	for _, c := range columns {
		if strings.EqualFold(c, name) {
			return c
		}
	}
	return ""
}

func containsAll(have []string, want []string) bool {
	set := make(map[string]struct{}, len(have))
	for _, h := range have {
		set[h] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[strings.ToLower(w)]; !ok {
			return false
		}
	}
	return true
}
