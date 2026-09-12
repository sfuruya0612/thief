package datadog

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

// MetricPoint は時系列の 1 点。
type MetricPoint struct {
	// T はエポックミリ秒。
	T int64 `json:"t"`
	// V は値。欠測は null のまま返す。0 に潰すと「値が 0 だった」と読めてしまう。
	V *float64 `json:"v"`
}

// MetricSeries はクエリが返した 1 つの時系列。
type MetricSeries struct {
	// Name は凡例に出す名前。Datadog が返す式 (expression) をそのまま使う。
	Name string `json:"name"`
	// Scope は系列を区別するタグ ("host:web-1,env:prod" など)。
	Scope string `json:"scope"`
	// Unit は値の単位の略記 ("B/s" など)。単位が無いメトリクスでは空になる。
	Unit   string        `json:"unit"`
	Points []MetricPoint `json:"points"`
}

// MetricQueryResult は 1 つのクエリの実行結果。
type MetricQueryResult struct {
	Query  string         `json:"query"`
	Series []MetricSeries `json:"series"`
}

// QueryMetrics runs a metrics query over the given time window
// (GET /api/v1/query).
//
// from と to は Datadog へ Unix 秒として渡す。ウィジェットのクエリ文字列
// (timeseries / query_value の q) をそのまま実行するために使う。
func QueryMetrics(ctx context.Context, api *MetricsV1API, query string, from, to time.Time) (MetricQueryResult, error) {
	resp, _, err := api.api.QueryMetrics(ctx, from.Unix(), to.Unix(), query)
	if err != nil {
		return MetricQueryResult{}, fmt.Errorf("query datadog metrics %q: %w", query, err)
	}

	// Datadog はクエリ自体の誤り (未知のメトリクス、構文エラー) を HTTP 200 と
	// status:"error" の組み合わせで返す。呼び出し側が成功と取り違えないよう、
	// ここでエラーへ変換する。
	if msg := resp.GetError(); msg != "" {
		return MetricQueryResult{}, fmt.Errorf("query datadog metrics %q: %s", query, msg)
	}

	result := MetricQueryResult{Query: query, Series: []MetricSeries{}}
	if q := resp.GetQuery(); q != "" {
		result.Query = q
	}
	for _, s := range resp.GetSeries() {
		result.Series = append(result.Series, MetricSeries{
			Name:   seriesName(s),
			Scope:  s.GetScope(),
			Unit:   seriesUnit(s.GetUnit()),
			Points: seriesPoints(s.GetPointlist()),
		})
	}
	return result, nil
}

// seriesName は系列の凡例名を決める。Datadog が返す expression
// ("avg:system.cpu.user{host:web-1}") が最も区別が付くのでこれを優先し、
// 無い場合はメトリクス名とスコープから組み立てる。
func seriesName(s datadogV1.MetricsQueryMetadata) string {
	if e := s.GetExpression(); e != "" {
		return e
	}
	metric := s.GetMetric()
	if metric == "" {
		metric = s.GetDisplayName()
	}
	scope := s.GetScope()
	switch {
	case metric != "" && scope != "":
		return metric + "{" + scope + "}"
	case metric != "":
		return metric
	case scope != "":
		return scope
	default:
		return ""
	}
}

// seriesUnit は単位の略記を組み立てる。Datadog は単位を 2 要素で返すことがあり、
// 1 つ目が主単位、2 つ目が分母 ("bytes per second" の "second") を表す。分母を
// 落とすと "B/s" を "B" と表示してしまい、値の意味が変わるため両方を使う。
func seriesUnit(units []datadogV1.MetricsQueryUnit) string {
	name := func(u datadogV1.MetricsQueryUnit) string {
		if s := u.GetShortName(); s != "" {
			return s
		}
		return u.GetName()
	}
	if len(units) == 0 {
		return ""
	}
	unit := name(units[0])
	if len(units) > 1 {
		if per := name(units[1]); per != "" {
			return unit + "/" + per
		}
	}
	return unit
}

// seriesPoints は pointlist ([[時刻, 値], ...]) を MetricPoint 列へ写す。
//
// 値が null の点は欠測として V を nil のまま残す。時刻を持たない点は時系列上に
// 置けないので落とす。
func seriesPoints(pointlist [][]*float64) []MetricPoint {
	points := []MetricPoint{}
	for _, p := range pointlist {
		if len(p) < 2 || p[0] == nil {
			continue
		}
		points = append(points, MetricPoint{T: int64(*p[0]), V: p[1]})
	}
	return points
}
