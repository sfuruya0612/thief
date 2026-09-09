package aws

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"golang.org/x/sync/errgroup"
)

// CostResource represents a line item in Cost Explorer results.
// Service は GroupByDimension で指定した次元の値 (デフォルトはサービス名) を保持する。
// AccountName は GroupByDimension が LINKED_ACCOUNT のときだけ、Service (アカウント ID) に
// 対応するアカウント名を保持する。他の次元では常に空文字。名前が取得できなかった ID も空文字。
// 表示用の文字列であり、ResourceID() と ResourceName() には含めない (識別子を表示名で揺らさない)。
type CostResource struct {
	TimePeriod         string  `json:"time_period"`
	Service            string  `json:"service"`
	AccountName        string  `json:"account_name"`
	UnblendedAmount    float64 `json:"unblended_amount"`
	NetAmortizedAmount float64 `json:"net_amortized_amount"`
	Unit               string  `json:"unit"`
}

func (r CostResource) ResourceID() string    { return r.TimePeriod + "/" + r.Service }
func (r CostResource) ResourceName() string  { return r.Service }
func (r CostResource) ResourceState() string { return "active" }
func (r CostResource) ServiceName() string   { return "cost" }

// ForecastResource represents a cost forecast entry.
type ForecastResource struct {
	TimePeriod string  `json:"time_period"`
	Amount     float64 `json:"amount"`
	Unit       string  `json:"unit"`
}

// CostGroupByDimension は GetCost の GroupBy 次元として許可する値。
// AWS Cost Explorer が対応する Dimension のうち、コスト可視化で使う頻度が高いものに限定する (YAGNI)。
const (
	CostGroupByService       = "SERVICE"
	CostGroupByUsageType     = "USAGE_TYPE"
	CostGroupByLinkedAccount = "LINKED_ACCOUNT"
	CostGroupByRegion        = "REGION"
)

// CostQueryOptions は GetCost の検索条件を表す。ゼロ値は以下のデフォルトとして扱う。
//   - Granularity: 空文字は DAILY
//   - GroupByDimension: 空文字は SERVICE
//   - Keyword: 空文字 (前後の空白を除いた結果が空の場合を含む) は絞り込みなし。
//     指定した場合は SERVICE / USAGE_TYPE / LINKED_ACCOUNT の 3 次元の値一覧に対して
//     大文字小文字を区別しない部分一致で照合し、一致した値だけを EQUALS で絞り込む。
//     LINKED_ACCOUNT はアカウント ID とアカウント名 (Attributes の description) の両方を照合する。
//   - StartDate/EndDate: 両方指定時のみ有効な期間として使う (YYYY-MM-DD)。指定時は Months を無視する。
//   - Months: StartDate/EndDate 未指定時のみ使う。0 以下は 1 (取得期間を遡る月数)
//
// GroupByDimension (結果のグルーピング次元) と Keyword (絞り込み条件) は独立した概念であり、
// GroupByDimension=USAGE_TYPE のまま Keyword がサービス名に一致してもよい。
type CostQueryOptions struct {
	IncludeToday     bool
	Granularity      string
	GroupByDimension string
	Keyword          string
	StartDate        string
	EndDate          string
	Months           int
}

// costExplorerAPI は Cost Explorer SDK クライアントのうち本パッケージが利用する操作の集合。
// テストでは手書きフェイクを差し込む。
type costExplorerAPI interface {
	GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
	GetDimensionValues(ctx context.Context, params *costexplorer.GetDimensionValuesInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error)
}

// costKeywordDimensions は Keyword の照合対象となる次元。Filter に並べる Or の要素順もこの順序に従う。
// REGION を含めないのは、要望が Service / Usage type / Linked account の 3 次元を対象としているため。
var costKeywordDimensions = []cetypes.Dimension{
	cetypes.DimensionService,
	cetypes.DimensionUsageType,
	cetypes.DimensionLinkedAccount,
}

// costAccountNameAttribute は GetDimensionValues が LINKED_ACCOUNT の応答で
// アカウント名を格納する Attributes のキー。
const costAccountNameAttribute = "description"

func costGranularity(g string) cetypes.Granularity {
	if g == "MONTHLY" {
		return cetypes.GranularityMonthly
	}
	return cetypes.GranularityDaily
}

func costGroupByDimension(dim string) string {
	switch dim {
	case CostGroupByUsageType, CostGroupByLinkedAccount, CostGroupByRegion:
		return dim
	default:
		return CostGroupByService
	}
}

// costAmount は Cost Explorer が返す金額文字列を float64 に変換する。
// nil またはパース不能な文字列は 0 として扱う (fmt.Sscanf と異なり部分一致は受理しない)。
func costAmount(s *string) float64 {
	if s == nil {
		return 0
	}
	v, err := strconv.ParseFloat(*s, 64)
	if err != nil {
		return 0
	}
	return v
}

// costDateRange は CostQueryOptions から Cost Explorer に渡す期間 (YYYY-MM-DD) を決める。
// StartDate/EndDate が両方指定されていればそれを使い、そうでなければ Months (現在からの
// 相対期間、デフォルト 1 ヶ月) から算出する。
func costDateRange(opts CostQueryOptions) (start, end string) {
	if opts.StartDate != "" && opts.EndDate != "" {
		return opts.StartDate, opts.EndDate
	}

	now := time.Now().UTC()
	end = now.Format("2006-01-02")
	if !opts.IncludeToday {
		end = now.AddDate(0, 0, -1).Format("2006-01-02")
	}
	months := opts.Months
	if months <= 0 {
		months = 1
	}
	start = now.AddDate(0, -months, 0).Format("2006-01-02")
	return start, end
}

// costDimensionFilter は単一次元の完全一致 (EQUALS) フィルタ式を組み立てる。
func costDimensionFilter(key cetypes.Dimension, values []string) cetypes.Expression {
	return cetypes.Expression{
		Dimensions: &cetypes.DimensionValues{
			Key:          key,
			Values:       values,
			MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionEquals},
		},
	}
}

// costDimensionValues は 1 次元の値一覧を NextPageToken が空になるまで取得する。
// SortBy は指定しない (SortBy を指定すると NextPageToken が使えず全ページを辿れないため)。
func costDimensionValues(ctx context.Context, client costExplorerAPI, dim cetypes.Dimension, start, end string) ([]cetypes.DimensionValuesWithAttributes, error) {
	var values []cetypes.DimensionValuesWithAttributes
	var token *string
	for {
		out, err := client.GetDimensionValues(ctx, &costexplorer.GetDimensionValuesInput{
			Dimension: dim,
			TimePeriod: &cetypes.DateInterval{
				Start: aws.String(start),
				End:   aws.String(end),
			},
			NextPageToken: token,
		})
		if err != nil {
			return nil, err
		}
		values = append(values, out.DimensionValues...)
		if out.NextPageToken == nil || *out.NextPageToken == "" {
			return values, nil
		}
		token = out.NextPageToken
	}
}

// costMatchDimensionValues は値一覧のうち、キーワードに大文字小文字を区別せず部分一致した値を返す。
// lowerKeyword は小文字化済みのキーワードを受け取る。LINKED_ACCOUNT は Value (アカウント ID) に
// 加えて Attributes["description"] (アカウント名) も照合し、どちらかが一致したら Value を返す。
// Attributes が nil または description キーを持たない場合は Value だけを照合する。
// Value が空の値は Filter の値として意味を持たないため除く。
func costMatchDimensionValues(dim cetypes.Dimension, values []cetypes.DimensionValuesWithAttributes, lowerKeyword string) []string {
	var matched []string
	for _, v := range values {
		value := ptrStr(v.Value)
		if value == "" {
			continue
		}
		if strings.Contains(strings.ToLower(value), lowerKeyword) {
			matched = append(matched, value)
			continue
		}
		if dim != cetypes.DimensionLinkedAccount {
			continue
		}
		// nil マップの索引はゼロ値と ok=false を返すため、Attributes が nil でも panic しない。
		if name, ok := v.Attributes[costAccountNameAttribute]; ok && strings.Contains(strings.ToLower(name), lowerKeyword) {
			matched = append(matched, value)
		}
	}
	return matched
}

// costAccountNames は LINKED_ACCOUNT の値一覧を取得し、アカウント ID (Value) からアカウント名
// (Attributes["description"]) への対応表を返す。Value が空文字の値は対応表に入れない
// (costMatchDimensionValues と同じ扱い。入れると Keys が空で Service が空文字の CostResource に
// 名前が付く)。同じ Value が複数回 (複数ページにまたがる場合を含む) 現れたら、後に処理した値の
// description で上書きする。Attributes が nil または description キーを持たない値は空文字になる
// (nil マップの索引はゼロ値を返すため panic しない)。
// キーワード解決 (costResolveKeyword) の取得結果は再利用しない。絞り込みと表示名の取得を
// 結合すると、片方の変更が他方の挙動を変えるため。
func costAccountNames(ctx context.Context, client costExplorerAPI, start, end string) (map[string]string, error) {
	values, err := costDimensionValues(ctx, client, cetypes.DimensionLinkedAccount, start, end)
	if err != nil {
		return nil, fmt.Errorf("get linked account names: %w", err)
	}
	names := make(map[string]string, len(values))
	for _, v := range values {
		id := ptrStr(v.Value)
		if id == "" {
			continue
		}
		names[id] = v.Attributes[costAccountNameAttribute]
	}
	return names, nil
}

// costHasGroups は GetCostAndUsage の結果に Groups が 1 つ以上あるかを返す。
func costHasGroups(results []cetypes.ResultByTime) bool {
	for _, result := range results {
		if len(result.Groups) > 0 {
			return true
		}
	}
	return false
}

// costResolveKeyword は costKeywordDimensions の各次元の値一覧を取得し、キーワードに部分一致した
// 値を次元ごとに返す。戻り値のスライスは costKeywordDimensions と同じ順序と長さを持つ。
// 3 次元の取得は互いに独立しているため errgroup で並列に実行し、各 goroutine は自分の index に
// のみ書き込む (データオーナーシップを goroutine ごとに分離するためロックは不要)。
// いずれかの次元が失敗したら部分的な結果を使わずエラーを返す。一致しなかった次元が
// 「一致無し」なのか「取得失敗」なのかを結果から区別できなくなるため。
func costResolveKeyword(ctx context.Context, client costExplorerAPI, keyword, start, end string) ([][]string, error) {
	lowerKeyword := strings.ToLower(keyword)
	matched := make([][]string, len(costKeywordDimensions))

	g, gctx := errgroup.WithContext(ctx)
	for i, dim := range costKeywordDimensions {
		g.Go(func() error {
			values, err := costDimensionValues(gctx, client, dim, start, end)
			if err != nil {
				return err
			}
			matched[i] = costMatchDimensionValues(dim, values, lowerKeyword)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("get dimension values: %w", err)
	}
	return matched, nil
}

// costKeywordFilter は次元ごとの一致値から GetCostAndUsage に渡す Filter を組み立てる。
// 一致が 1 次元だけならその Dimensions をルートに持つ Expression を、2 次元以上なら Or に
// まとめた Expression を返す。一致が無い次元は式に含めない。3 次元とも一致が無ければ nil を返す。
// cetypes.Expression は 1 つのフィルタ機構のみを持つ想定であるため、トップレベルの Expression に
// Dimensions と Or を同時に設定しない。
func costKeywordFilter(matched [][]string) *cetypes.Expression {
	var exprs []cetypes.Expression
	for i, values := range matched {
		if len(values) == 0 {
			continue
		}
		exprs = append(exprs, costDimensionFilter(costKeywordDimensions[i], values))
	}
	switch len(exprs) {
	case 0:
		return nil
	case 1:
		// スライスの要素そのものへのポインタを返さず、値をコピーしてから参照を返す。
		single := exprs[0]
		return &single
	default:
		return &cetypes.Expression{Or: exprs}
	}
}

// costFilter は Keyword から GetCostAndUsage に渡す Filter を組み立てる。
// 戻り値の filter は nil でも意味が 2 つあるため、絞り込みの結果が空に確定したかを skip で返す。
//   - キーワードが空 (前後の空白を除いた結果が空の場合を含む): GetDimensionValues を呼ばず
//     (nil, false, nil) を返す。絞り込みなしで全件を取得する
//   - キーワードがどの次元にも一致しない: (nil, true, nil) を返す。結果が空になることが確定して
//     いるため、呼び出し側は課金される GetCostAndUsage を呼ばない
func costFilter(ctx context.Context, client costExplorerAPI, keyword, start, end string) (filter *cetypes.Expression, skip bool, err error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, false, nil
	}

	matched, err := costResolveKeyword(ctx, client, keyword, start, end)
	if err != nil {
		return nil, false, err
	}

	f := costKeywordFilter(matched)
	if f == nil {
		return nil, true, nil
	}
	return f, false, nil
}

// GetCost returns cost grouped by the given dimension for the given date range.
// If includeToday is false, the end date is yesterday.
func GetCost(ctx context.Context, profile, region string, opts CostQueryOptions) ([]CostResource, error) {
	// Cost Explorer is a global service; us-east-1 is the standard endpoint.
	client, err := newCostExplorerClient(ctx, profile, "us-east-1")
	if err != nil {
		return nil, err
	}
	return getCost(ctx, client, opts)
}

// getCost は Cost Explorer クライアントを受け取り、期間とフィルタを組み立てて結果を変換する。
// テストでは costExplorerAPI の手書きフェイクを差し込む。
func getCost(ctx context.Context, client costExplorerAPI, opts CostQueryOptions) ([]CostResource, error) {
	start, end := costDateRange(opts)

	filter, skip, err := costFilter(ctx, client, opts.Keyword, start, end)
	if err != nil {
		return nil, err
	}
	if skip {
		return nil, nil
	}

	input := &costexplorer.GetCostAndUsageInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(start),
			End:   aws.String(end),
		},
		Granularity: costGranularity(opts.Granularity),
		Metrics:     []string{"UnblendedCost", "NetAmortizedCost"},
		GroupBy: []cetypes.GroupDefinition{
			{Type: cetypes.GroupDefinitionTypeDimension, Key: aws.String(costGroupByDimension(opts.GroupByDimension))},
		},
		Filter: filter,
	}

	out, err := client.GetCostAndUsage(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("get cost and usage: %w", err)
	}

	// Group by が LINKED_ACCOUNT のとき Keys[0] はアカウント ID なので、表示用のアカウント名を
	// GetDimensionValues から引く。Groups が 1 つも無ければ名前を引く相手がいないため、リクエスト
	// ごとに課金される GetDimensionValues を呼ばない。他の次元では呼ばず、nil マップの索引で
	// AccountName は常に空文字になる。
	// GetCostAndUsage と並列にしない案は、Groups が空のときに呼び出しを省けなくなるため採らない。
	// 名前の取得に失敗したら ID だけで縮退せずエラーを返す (costResolveKeyword と同じ扱い)。
	var accountNames map[string]string
	if costGroupByDimension(opts.GroupByDimension) == CostGroupByLinkedAccount && costHasGroups(out.ResultsByTime) {
		accountNames, err = costAccountNames(ctx, client, start, end)
		if err != nil {
			return nil, err
		}
	}

	var resources []CostResource
	for _, result := range out.ResultsByTime {
		period := ""
		if result.TimePeriod != nil {
			period = ptrStr(result.TimePeriod.Start)
		}
		for _, group := range result.Groups {
			service := ""
			if len(group.Keys) > 0 {
				service = group.Keys[0]
			}
			unblended := 0.0
			netAmortized := 0.0
			unit := ""
			if m, ok := group.Metrics["UnblendedCost"]; ok {
				unblended = costAmount(m.Amount)
				unit = ptrStr(m.Unit)
			}
			if m, ok := group.Metrics["NetAmortizedCost"]; ok {
				netAmortized = costAmount(m.Amount)
				if unit == "" {
					unit = ptrStr(m.Unit)
				}
			}
			resources = append(resources, CostResource{
				TimePeriod:         period,
				Service:            service,
				AccountName:        accountNames[service],
				UnblendedAmount:    unblended,
				NetAmortizedAmount: netAmortized,
				Unit:               unit,
			})
		}
	}
	return resources, nil
}

// GetForecast returns the cost forecast for the current month.
func GetForecast(ctx context.Context, profile, _ string) ([]ForecastResource, error) {
	client, err := newCostExplorerClient(ctx, profile, "us-east-1")
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	start := now.Format("2006-01-02")
	// End of current month.
	nextMonth := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	end := nextMonth.Format("2006-01-02")

	out, err := client.GetCostForecast(ctx, &costexplorer.GetCostForecastInput{
		TimePeriod: &cetypes.DateInterval{
			Start: aws.String(start),
			End:   aws.String(end),
		},
		Granularity: cetypes.GranularityMonthly,
		Metric:      cetypes.MetricBlendedCost,
	})
	if err != nil {
		return nil, fmt.Errorf("get cost forecast: %w", err)
	}

	var resources []ForecastResource
	for _, result := range out.ForecastResultsByTime {
		period := ""
		if result.TimePeriod != nil {
			period = ptrStr(result.TimePeriod.Start)
		}
		amount := costAmount(result.MeanValue)
		unit := ""
		if out.Total != nil {
			unit = ptrStr(out.Total.Unit)
		}
		resources = append(resources, ForecastResource{
			TimePeriod: period,
			Amount:     amount,
			Unit:       unit,
		})
	}
	return resources, nil
}

// newCostExplorerClient は Cost Explorer API クライアントを生成する。
func newCostExplorerClient(ctx context.Context, profile, region string) (*costexplorer.Client, error) {
	return NewClient(ctx, profile, region, func(cfg aws.Config) *costexplorer.Client {
		return costexplorer.NewFromConfig(cfg)
	})
}
