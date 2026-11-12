package accesslog

import (
	"fmt"
	"sort"
	"time"
)

// Merge はソースごとの集計結果をマージする。
// WAF はリクエスト数の加算対象にせず、ブロックとチャレンジのオーバーレイとして扱う。
// CloudFront の origin の ALB は、CloudFront が選ばれている場合に IP の集計から外す。
func Merge(results []SourceResult, sources []Source) Result {
	byID := make(map[string]Source, len(sources))
	selected := make(map[string]bool, len(sources))
	for _, source := range sources {
		byID[source.ID] = source
		selected[source.ID] = true
	}

	var warnings []string
	pairAcc := make(map[string]*pairAccumulator)
	ipRows := make([]IPSummary, 0)
	sourceSummaries := make([]SourceSummary, 0, len(results))
	excludedOrigins := 0

	for _, result := range results {
		source, ok := byID[result.SourceID]
		if !ok {
			continue
		}
		if result.Err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", source.Name, result.Err))
			continue
		}
		excluded := source.Type == SourceALB && source.OriginOf != "" && selected[source.OriginOf]
		if excluded {
			excludedOrigins++
		}
		summary := SourceSummary{
			SourceID:       source.ID,
			Records:        sumPairRequests(result.Rows),
			BytesScanned:   result.BytesScanned,
			RecordsScanned: result.RecordsScanned,
		}
		for _, row := range result.Rows {
			summary.Blocked += row.BlockedCount
			summary.Challenged += row.ChallengedCount
			summary.ClientErrors += row.ClientErrorCount
			summary.ServerErrors += row.ServerErrorCount
		}
		sourceSummaries = append(sourceSummaries, summary)

		if excluded {
			// origin の ALB は IP の集計に加算しない。ソース別の統計だけを残す。
			continue
		}
		for _, row := range result.Rows {
			key := row.ClientIP + "\x00" + row.URIPath
			acc, ok := pairAcc[key]
			if !ok {
				acc = &pairAccumulator{row: PairRow{ClientIP: row.ClientIP, URIPath: row.URIPath}}
				pairAcc[key] = acc
			}
			acc.add(source, row)
		}
		for _, row := range result.IPs {
			row.Sources = []string{source.ID}
			row.SourceCounts = map[string]int64{source.ID: row.RequestCount}
			ipRows = append(ipRows, row)
		}
	}

	rows := make([]PairRow, 0, len(pairAcc))
	for _, acc := range pairAcc {
		row := acc.finish()
		rows = append(rows, row)
	}
	sortPairRows(rows)
	if len(rows) > MaxLimit {
		rows = rows[:MaxLimit]
	}
	ipRows = filterAndSortIPs(ipRows)

	summary := Summary{
		SourceCount: len(sourceSummaries),
		BySources:   sourceSummaries,
	}
	for _, row := range rows {
		summary.RecordCount += row.RequestCount
		summary.BlockedCount += row.BlockedCount
		summary.ChallengedCount += row.ChallengedCount
		summary.ClientErrorCount += row.ClientErrorCount
		summary.ServerErrorCount += row.ServerErrorCount
	}
	ips := make(map[string]struct{})
	uris := make(map[string]struct{})
	for _, row := range rows {
		ips[row.ClientIP] = struct{}{}
		uris[row.URIPath] = struct{}{}
	}
	summary.DistinctIPs = len(ips)
	summary.DistinctURIs = len(uris)
	summary.DuplicateWarning = len(sourceSummaries) > 1
	if summary.DuplicateWarning {
		warnings = append(warnings, "counts are log records; the same request may appear in CloudFront, WAF, and ALB logs")
	}
	if excludedOrigins > 0 {
		warnings = append(warnings, "origin ALB sources are excluded from the client IP aggregation because CloudFront is the edge")
	}
	for _, source := range sources {
		if source.Type == SourceWAF && selected[source.ID] {
			warnings = append(warnings, "WAF actions are overlaid on the request counts and are not added to them")
			break
		}
	}

	return Result{
		Rows:     rows,
		IPs:      ipRows,
		Summary:  summary,
		Warnings: dedupeStrings(warnings),
	}
}

type pairAccumulator struct {
	row            PairRow
	nonWAFRequests int64
	wafRequests    int64
	nonWAFBlocked  int64
	wafBlocked     int64
	challenged     int64
	countMatched   int64
	clientErrors   int64
	serverErrors   int64
	sourceCounts   map[string]int64
	sources        map[string]struct{}
	hasWAF         bool
	hasNonWAF      bool
	firstSeen      *time.Time
	lastSeen       *time.Time
	userAgent      string
	userAgentAt    *time.Time
	ruleID         string
	ruleIDAt       *time.Time
	hasQuery       bool
	sampleQuery    string
	sampleQueryAt  *time.Time
}

func (a *pairAccumulator) add(source Source, row PairRow) {
	if a.sourceCounts == nil {
		a.sourceCounts = make(map[string]int64)
		a.sources = make(map[string]struct{})
	}
	a.sourceCounts[source.ID] += row.RequestCount
	a.sources[source.ID] = struct{}{}
	a.firstSeen = minTime(a.firstSeen, row.FirstSeen)
	a.lastSeen = maxTime(a.lastSeen, row.LastSeen)
	if source.Type == SourceWAF {
		a.hasWAF = true
		a.wafRequests += row.RequestCount
		a.wafBlocked += row.BlockedCount
		a.challenged += row.ChallengedCount
		a.countMatched += row.CountMatched
	} else {
		a.hasNonWAF = true
		a.nonWAFRequests += row.RequestCount
		a.clientErrors += row.ClientErrorCount
		a.serverErrors += row.ServerErrorCount
		a.nonWAFBlocked += row.BlockedCount
	}
	if row.SampleUserAgent != "" && (a.userAgent == "" || after(row.LastSeen, a.userAgentAt)) {
		a.userAgent = row.SampleUserAgent
		a.userAgentAt = row.LastSeen
	}
	if row.SampleRuleID != "" && (a.ruleID == "" || after(row.LastSeen, a.ruleIDAt)) {
		a.ruleID = row.SampleRuleID
		a.ruleIDAt = row.LastSeen
	}
	if row.HasQuery {
		a.hasQuery = true
	}
	if row.SampleQuery != "" && (a.sampleQuery == "" || after(row.LastSeen, a.sampleQueryAt)) {
		a.sampleQuery = row.SampleQuery
		a.sampleQueryAt = row.LastSeen
	}
}

func (a *pairAccumulator) finish() PairRow {
	row := a.row
	if a.hasNonWAF {
		row.RequestCount = a.nonWAFRequests
	} else {
		row.RequestCount = a.wafRequests
	}
	// WAF のソースがある場合は WAF のブロックを正とし、ALB の waf アクションとの
	// 二重計上を避ける。WAF が無い場合は ALB の waf アクションを使う。
	if a.hasWAF {
		row.BlockedCount = a.wafBlocked
	} else {
		row.BlockedCount = a.nonWAFBlocked
	}
	row.ChallengedCount = a.challenged
	row.CountMatched = a.countMatched
	row.ClientErrorCount = a.clientErrors
	row.ServerErrorCount = a.serverErrors
	row.FirstSeen = a.firstSeen
	row.LastSeen = a.lastSeen
	row.SampleUserAgent = a.userAgent
	row.SampleRuleID = a.ruleID
	row.HasQuery = a.hasQuery
	row.SampleQuery = truncate(a.sampleQuery, 512)
	if len(a.sources) == 1 {
		for id := range a.sources {
			row.Sources = []string{id}
		}
	} else {
		row.Sources = sortedKeys(a.sources)
	}
	row.SourceCounts = a.sourceCounts
	row.Flags = flagsForPair(row)
	return row
}

func flagsForPair(row PairRow) []string {
	var flags []string
	if row.BlockedCount > 0 {
		flags = append(flags, "blocked")
	}
	if row.ChallengedCount > 0 {
		flags = append(flags, "challenged")
	}
	if row.CountMatched > 0 {
		flags = append(flags, "count_matched")
	}
	if row.ClientErrorCount > 0 {
		flags = append(flags, "client_error")
	}
	if row.ServerErrorCount > 0 {
		flags = append(flags, "server_error")
	}
	if IsSensitivePath(row.URIPath) {
		flags = append(flags, "sensitive_path")
	}
	if row.HasQuery {
		flags = append(flags, "has_query")
	}
	if len(row.Sources) > 1 {
		flags = append(flags, "shared")
	}
	return flags
}

func sortPairRows(rows []PairRow) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.BlockedCount != b.BlockedCount {
			return a.BlockedCount > b.BlockedCount
		}
		if a.ChallengedCount != b.ChallengedCount {
			return a.ChallengedCount > b.ChallengedCount
		}
		if a.ClientErrorCount != b.ClientErrorCount {
			return a.ClientErrorCount > b.ClientErrorCount
		}
		if a.RequestCount != b.RequestCount {
			return a.RequestCount > b.RequestCount
		}
		if !sameTime(a.LastSeen, b.LastSeen) {
			return after(a.LastSeen, b.LastSeen)
		}
		if a.ClientIP != b.ClientIP {
			return a.ClientIP < b.ClientIP
		}
		return a.URIPath < b.URIPath
	})
}

func filterAndSortIPs(rows []IPSummary) []IPSummary {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.BlockedCount != b.BlockedCount {
			return a.BlockedCount > b.BlockedCount
		}
		if a.ChallengedCount != b.ChallengedCount {
			return a.ChallengedCount > b.ChallengedCount
		}
		if a.ClientErrorCount != b.ClientErrorCount {
			return a.ClientErrorCount > b.ClientErrorCount
		}
		if a.RequestCount != b.RequestCount {
			return a.RequestCount > b.RequestCount
		}
		return a.ClientIP < b.ClientIP
	})
	if len(rows) > MaxLimit {
		rows = rows[:MaxLimit]
	}
	for i := range rows {
		rows[i].Flags = flagsForIP(rows[i])
	}
	return rows
}

func flagsForIP(row IPSummary) []string {
	var flags []string
	if row.BlockedCount > 0 {
		flags = append(flags, "blocked")
	}
	if row.ChallengedCount > 0 {
		flags = append(flags, "challenged")
	}
	if row.ClientErrorCount > 0 {
		flags = append(flags, "client_error")
	}
	if row.ServerErrorCount > 0 {
		flags = append(flags, "server_error")
	}
	if row.DistinctURIs >= 50 {
		flags = append(flags, "many_uris")
	}
	return flags
}

func sumPairRequests(rows []PairRow) int64 {
	var total int64
	for _, row := range rows {
		total += row.RequestCount
	}
	return total
}

func minTime(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if b.Before(*a) {
		return b
	}
	return a
}

func maxTime(a, b *time.Time) *time.Time {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if b.After(*a) {
		return b
	}
	return a
}

func after(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
