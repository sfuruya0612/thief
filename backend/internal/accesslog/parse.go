package accesslog

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseAthenaRows は Athena の結果を集計行に変換する。
// view は "pairs" か "ips" のいずれかとする。
func ParseAthenaRows(columns []string, rows [][]string, view string, sourceKeyToID map[string]string, defaultSourceID string) ([]PairRow, []IPSummary, error) {
	index := make(map[string]int, len(columns))
	for i, column := range columns {
		index[strings.ToLower(column)] = i
	}
	switch view {
	case "pairs":
		out := make([]PairRow, 0, len(rows))
		for _, row := range rows {
			pair, err := pairRowFromAthena(row, index, sourceKeyToID, defaultSourceID)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, pair)
		}
		return out, nil, nil
	case "ips":
		out := make([]IPSummary, 0, len(rows))
		for _, row := range rows {
			ip, err := ipSummaryFromAthena(row, index, sourceKeyToID, defaultSourceID)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, ip)
		}
		return nil, out, nil
	default:
		return nil, nil, fmt.Errorf("parse athena rows: unknown view %q", view)
	}
}

func pairRowFromAthena(row []string, index map[string]int, sourceKeyToID map[string]string, defaultSourceID string) (PairRow, error) {
	sourceKey := cell(row, index, "source_key")
	out := PairRow{
		SourceKey:        sourceKey,
		ClientIP:         cell(row, index, "client_ip"),
		URIPath:          cell(row, index, "uri_path"),
		RequestCount:     cellInt(row, index, "request_count"),
		BlockedCount:     cellInt(row, index, "blocked_count"),
		ChallengedCount:  cellInt(row, index, "challenged_count"),
		CountMatched:     cellInt(row, index, "count_matched"),
		ClientErrorCount: cellInt(row, index, "client_error_count"),
		ServerErrorCount: cellInt(row, index, "server_error_count"),
		SampleUserAgent:  cell(row, index, "sample_user_agent"),
		SampleRuleID:     cell(row, index, "sample_rule_id"),
		HasQuery:         cellInt(row, index, "has_query") != 0,
		SampleQuery:      cell(row, index, "sample_query"),
	}
	if out.SampleQuery == "-" {
		out.SampleQuery = ""
	}
	out.FirstSeen = cellTime(row, index, "first_seen_ms")
	out.LastSeen = cellTime(row, index, "last_seen_ms")
	out.SourceID = sourceIDForRow(sourceKey, sourceKeyToID, defaultSourceID)
	return out, nil
}

func ipSummaryFromAthena(row []string, index map[string]int, sourceKeyToID map[string]string, defaultSourceID string) (IPSummary, error) {
	sourceKey := cell(row, index, "source_key")
	out := IPSummary{
		SourceKey:        sourceKey,
		ClientIP:         cell(row, index, "client_ip"),
		RequestCount:     cellInt(row, index, "request_count"),
		BlockedCount:     cellInt(row, index, "blocked_count"),
		ChallengedCount:  cellInt(row, index, "challenged_count"),
		CountMatched:     cellInt(row, index, "count_matched"),
		ClientErrorCount: cellInt(row, index, "client_error_count"),
		ServerErrorCount: cellInt(row, index, "server_error_count"),
		DistinctURIs:     cellInt(row, index, "distinct_uris"),
	}
	out.FirstSeen = cellTime(row, index, "first_seen_ms")
	out.LastSeen = cellTime(row, index, "last_seen_ms")
	out.SourceID = sourceIDForRow(sourceKey, sourceKeyToID, defaultSourceID)
	return out, nil
}

// ParseInsightsRows は Logs Insights の集計行を PairRow に変換する。
// action ごとに返る行を (source, ip, uri) でまとめる。
func ParseInsightsRows(results []map[string]string, sourceKeyToID map[string]string, defaultSourceID string) []PairRow {
	type key struct {
		sourceKey string
		ip        string
		uri       string
	}
	index := make(map[key]*PairRow)
	for _, row := range results {
		sourceKey := row["webaclId"]
		if sourceKey == "" {
			sourceKey = row["webaclid"]
		}
		ip := row["ip"]
		uri := row["uri"]
		if ip == "" && uri == "" {
			continue
		}
		k := key{sourceKey: sourceKey, ip: ip, uri: uri}
		pair, ok := index[k]
		if !ok {
			pair = &PairRow{
				SourceID:  sourceIDForRow(sourceKey, sourceKeyToID, defaultSourceID),
				SourceKey: sourceKey,
				ClientIP:  ip,
				URIPath:   uri,
			}
			index[k] = pair
		}
		count := parseInt64(row["request_count"])
		pair.RequestCount += count
		switch strings.ToUpper(row["action"]) {
		case "BLOCK":
			pair.BlockedCount += count
		case "CAPTCHA", "CHALLENGE":
			pair.ChallengedCount += count
		case "COUNT":
			pair.CountMatched += count
		}
		if first := parseMillis(row["first_seen_ms"]); first != nil {
			pair.FirstSeen = minTime(pair.FirstSeen, first)
		}
		if last := parseMillis(row["last_seen_ms"]); last != nil {
			pair.LastSeen = maxTime(pair.LastSeen, last)
		}
	}
	out := make([]PairRow, 0, len(index))
	for _, pair := range index {
		out = append(out, *pair)
	}
	return out
}

func cell(row []string, index map[string]int, name string) string {
	i, ok := index[strings.ToLower(name)]
	if !ok || i < 0 || i >= len(row) {
		return ""
	}
	value := row[i]
	if value == "NULL" {
		return ""
	}
	return value
}

func cellInt(row []string, index map[string]int, name string) int64 {
	return parseInt64(cell(row, index, name))
}

func cellTime(row []string, index map[string]int, name string) *time.Time {
	return parseMillis(cell(row, index, name))
}

func parseInt64(s string) int64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseMillis(s string) *time.Time {
	v := parseInt64(s)
	if v == 0 {
		return nil
	}
	t := time.UnixMilli(v).UTC()
	return &t
}

func sourceIDForRow(sourceKey string, sourceKeyToID map[string]string, defaultSourceID string) string {
	if sourceKey != "" {
		if id, ok := sourceKeyToID[sourceKey]; ok {
			return id
		}
	}
	return defaultSourceID
}

// SplitSourceResults は 1 回のクエリでまとめた行をソースごとの結果に分ける。
func SplitSourceResults(rows []PairRow, ips []IPSummary, bytesScanned, recordsScanned int64) []SourceResult {
	byID := make(map[string]*SourceResult)
	get := func(id string) *SourceResult {
		if id == "" {
			id = "unknown"
		}
		result, ok := byID[id]
		if !ok {
			result = &SourceResult{SourceID: id, BytesScanned: bytesScanned, RecordsScanned: recordsScanned}
			byID[id] = result
		}
		return result
	}
	for _, row := range rows {
		result := get(row.SourceID)
		result.Rows = append(result.Rows, row)
	}
	for _, row := range ips {
		result := get(row.SourceID)
		result.IPs = append(result.IPs, row)
	}
	out := make([]SourceResult, 0, len(byID))
	for _, result := range byID {
		out = append(out, *result)
	}
	return out
}
