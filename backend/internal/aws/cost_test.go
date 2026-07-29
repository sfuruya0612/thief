package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestCostGranularity(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want cetypes.Granularity
	}{
		{name: "monthly", in: "MONTHLY", want: cetypes.GranularityMonthly},
		{name: "daily", in: "DAILY", want: cetypes.GranularityDaily},
		{name: "empty defaults to daily", in: "", want: cetypes.GranularityDaily},
		{name: "unknown defaults to daily", in: "HOURLY", want: cetypes.GranularityDaily},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := costGranularity(tt.in)
			if got != tt.want {
				t.Errorf("costGranularity(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestCostGroupByDimension(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "usage type", in: CostGroupByUsageType, want: CostGroupByUsageType},
		{name: "linked account", in: CostGroupByLinkedAccount, want: CostGroupByLinkedAccount},
		{name: "region", in: CostGroupByRegion, want: CostGroupByRegion},
		{name: "service", in: CostGroupByService, want: CostGroupByService},
		{name: "empty defaults to service", in: "", want: CostGroupByService},
		{name: "unknown defaults to service", in: "AZ", want: CostGroupByService},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := costGroupByDimension(tt.in)
			if got != tt.want {
				t.Errorf("costGroupByDimension(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCostAmount(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	tests := []struct {
		name string
		in   *string
		want float64
	}{
		{name: "decimal", in: strPtr("12.34"), want: 12.34},
		{name: "zero", in: strPtr("0"), want: 0},
		{name: "negative", in: strPtr("-0.5"), want: -0.5},
		{name: "exponent", in: strPtr("1.5e2"), want: 150},
		{name: "nil is zero", in: nil, want: 0},
		{name: "empty is zero", in: strPtr(""), want: 0},
		{name: "trailing garbage is zero", in: strPtr("12.34abc"), want: 0},
		{name: "non numeric is zero", in: strPtr("abc"), want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := costAmount(tt.in)
			if got != tt.want {
				t.Errorf("costAmount(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestCostDateRange(t *testing.T) {
	t.Run("StartDate/EndDate 両方指定時はそれを優先する", func(t *testing.T) {
		start, end := costDateRange(CostQueryOptions{
			StartDate: "2026-01-01",
			EndDate:   "2026-01-31",
			Months:    6, // 優先されないことを確認する
		})
		if start != "2026-01-01" || end != "2026-01-31" {
			t.Errorf("costDateRange() = (%q, %q), want (2026-01-01, 2026-01-31)", start, end)
		}
	})

	t.Run("StartDate のみでは Months ベースにフォールバックする", func(t *testing.T) {
		_, end := costDateRange(CostQueryOptions{StartDate: "2026-01-01"})
		wantEnd := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		if end != wantEnd {
			t.Errorf("end = %q, want %q (StartDate だけでは無効)", end, wantEnd)
		}
	})

	t.Run("IncludeToday が false の場合 end は前日になる", func(t *testing.T) {
		_, end := costDateRange(CostQueryOptions{IncludeToday: false})
		want := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		if end != want {
			t.Errorf("end = %q, want %q", end, want)
		}
	})

	t.Run("IncludeToday が true の場合 end は今日になる", func(t *testing.T) {
		_, end := costDateRange(CostQueryOptions{IncludeToday: true})
		want := time.Now().UTC().Format("2006-01-02")
		if end != want {
			t.Errorf("end = %q, want %q", end, want)
		}
	})

	t.Run("Months が 0 以下ならデフォルト 1 ヶ月遡る", func(t *testing.T) {
		start, _ := costDateRange(CostQueryOptions{Months: -1})
		want := time.Now().UTC().AddDate(0, -1, 0).Format("2006-01-02")
		if start != want {
			t.Errorf("start = %q, want %q", start, want)
		}
	})

	t.Run("Months 指定分だけ遡る", func(t *testing.T) {
		start, _ := costDateRange(CostQueryOptions{Months: 3})
		want := time.Now().UTC().AddDate(0, -3, 0).Format("2006-01-02")
		if start != want {
			t.Errorf("start = %q, want %q", start, want)
		}
	})
}

// fakeCostExplorer は costExplorerAPI の手書きフェイク。受け取った入力を記録し、
// あらかじめ設定した出力またはエラーを返す。
type fakeCostExplorer struct {
	gotInput *costexplorer.GetCostAndUsageInput
	out      *costexplorer.GetCostAndUsageOutput
	err      error
}

func (f *fakeCostExplorer) GetCostAndUsage(_ context.Context, p *costexplorer.GetCostAndUsageInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	f.gotInput = p
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

// dimensionExpr はテストの期待値として単一次元の EQUALS フィルタ式を組み立てる。
// 実装の costDimensionFilter とは独立に書き下すことで、実装の変更をテストが検出できるようにする。
func dimensionExpr(key cetypes.Dimension, value string) cetypes.Expression {
	return cetypes.Expression{
		Dimensions: &cetypes.DimensionValues{
			Key:          key,
			Values:       []string{value},
			MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionEquals},
		},
	}
}

// expressionCmpOpts は cetypes の構造体が持つ埋め込みの非公開シリアライズマーカーを比較対象から除く。
func expressionCmpOpts() cmp.Option {
	return cmpopts.IgnoreUnexported(cetypes.Expression{}, cetypes.DimensionValues{})
}

func TestGetCostFilter(t *testing.T) {
	const (
		testService = "AmazonEC2"
		testAccount = "123456789012"
	)
	serviceExpr := dimensionExpr(cetypes.DimensionService, testService)
	accountExpr := dimensionExpr(cetypes.DimensionLinkedAccount, testAccount)
	blankServiceExpr := dimensionExpr(cetypes.DimensionService, " ")

	tests := []struct {
		name        string
		opts        CostQueryOptions
		wantFilter  *cetypes.Expression
		wantGroupBy string
	}{
		{
			name:        "ServiceFilter のみ指定した場合は SERVICE の単一 Dimensions になる",
			opts:        CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", ServiceFilter: testService},
			wantFilter:  &serviceExpr,
			wantGroupBy: CostGroupByService,
		},
		{
			name:        "AccountFilter のみ指定した場合は LINKED_ACCOUNT の単一 Dimensions になる",
			opts:        CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", AccountFilter: testAccount},
			wantFilter:  &accountExpr,
			wantGroupBy: CostGroupByService,
		},
		{
			name: "両方指定した場合は And に 2 要素を持つ Expression になる",
			opts: CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", ServiceFilter: testService, AccountFilter: testAccount},
			wantFilter: &cetypes.Expression{
				And: []cetypes.Expression{serviceExpr, accountExpr},
			},
			wantGroupBy: CostGroupByService,
		},
		{
			name:        "両方未指定の場合は Filter を設定しない",
			opts:        CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31"},
			wantFilter:  nil,
			wantGroupBy: CostGroupByService,
		},
		{
			name: "GroupBy=LINKED_ACCOUNT と AccountFilter は同時に指定できる",
			opts: CostQueryOptions{
				StartDate:        "2026-07-01",
				EndDate:          "2026-07-31",
				GroupByDimension: CostGroupByLinkedAccount,
				AccountFilter:    testAccount,
			},
			wantFilter:  &accountExpr,
			wantGroupBy: CostGroupByLinkedAccount,
		},
		{
			// 直前の GroupBy=LINKED_ACCOUNT ケースと対称に、ServiceFilter 側でも GroupBy と Filter が
			// 独立に効くことを固定する。ただし costGroupByDimension は SERVICE を case に列挙せず
			// default で返すため、この経路は先頭の「ServiceFilter のみ」ケース (GroupByDimension が
			// 空文字) と同一の分岐を通り、生成される入力も一致する。したがって検出力は先頭ケースと
			// 重複しており、単独で落ちる実装上の欠陥は存在しない。仕様の回帰固定として残す。
			name: "GroupBy=SERVICE と ServiceFilter は同時に指定できる",
			opts: CostQueryOptions{
				StartDate:        "2026-07-01",
				EndDate:          "2026-07-31",
				GroupByDimension: CostGroupByService,
				ServiceFilter:    testService,
			},
			wantFilter:  &serviceExpr,
			wantGroupBy: CostGroupByService,
		},
		{
			// costGroupByDimension が未知の値を SERVICE にフォールバックすることを固定する。
			// 「未指定 (空文字)」ケースとは別に持つ意味がある。未指定のみを SERVICE に解決して
			// 未知の値はそのまま Cost Explorer に透過させる実装に変わった場合、空文字ケースは
			// 通ったままこのケースだけが落ちる。
			name: "未知の GroupBy 次元は SERVICE にフォールバックする",
			opts: CostQueryOptions{
				StartDate:        "2026-07-01",
				EndDate:          "2026-07-31",
				GroupByDimension: "BOGUS_DIMENSION",
				ServiceFilter:    testService,
			},
			wantFilter:  &serviceExpr,
			wantGroupBy: CostGroupByService,
		},
		{
			// GroupBy と Filter は独立した概念であり、絞り込む次元と集計する次元は一致しなくてよい。
			// GroupBy をデフォルト (SERVICE) 以外にしたうえで ServiceFilter を指定し、
			// GroupBy の値が Filter に混入しないこと、および ServiceFilter が GroupBy を
			// 上書きしないことの両方を確認する。
			name: "GroupBy=USAGE_TYPE と ServiceFilter は同時に指定できる",
			opts: CostQueryOptions{
				StartDate:        "2026-07-01",
				EndDate:          "2026-07-31",
				GroupByDimension: CostGroupByUsageType,
				ServiceFilter:    testService,
			},
			wantFilter:  &serviceExpr,
			wantGroupBy: CostGroupByUsageType,
		},
		{
			name: "GroupBy=REGION と ServiceFilter/AccountFilter の両方は同時に指定できる",
			opts: CostQueryOptions{
				StartDate:        "2026-07-01",
				EndDate:          "2026-07-31",
				GroupByDimension: CostGroupByRegion,
				ServiceFilter:    testService,
				AccountFilter:    testAccount,
			},
			wantFilter: &cetypes.Expression{
				And: []cetypes.Expression{serviceExpr, accountExpr},
			},
			wantGroupBy: CostGroupByRegion,
		},
		{
			// 空文字判定のみで絞り込みの有無を決めるため、空白のみの文字列は
			// 「絞り込みあり」として Cost Explorer に渡る。前後の空白除去は入力側 (frontend) の責務。
			name:        "空白のみの ServiceFilter は絞り込みありとして扱う",
			opts:        CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", ServiceFilter: " "},
			wantFilter:  &blankServiceExpr,
			wantGroupBy: CostGroupByService,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeCostExplorer{out: &costexplorer.GetCostAndUsageOutput{}}
			if _, err := getCost(context.Background(), fake, tt.opts); err != nil {
				t.Fatalf("getCost() error = %v, want nil", err)
			}
			if fake.gotInput == nil {
				t.Fatal("GetCostAndUsage was not called")
			}
			if diff := cmp.Diff(tt.wantFilter, fake.gotInput.Filter, expressionCmpOpts()); diff != "" {
				t.Errorf("Filter mismatch (-want +got):\n%s", diff)
			}
			if len(fake.gotInput.GroupBy) != 1 {
				t.Fatalf("GroupBy length = %d, want 1", len(fake.gotInput.GroupBy))
			}
			if got := ptrStr(fake.gotInput.GroupBy[0].Key); got != tt.wantGroupBy {
				t.Errorf("GroupBy key = %q, want %q", got, tt.wantGroupBy)
			}
			// 絞り込み条件の有無が Filter/GroupBy 以外のフィールドに影響しないことを確認する。
			// Metrics が欠けると UnblendedCost / NetAmortizedCost のどちらかが取得できなくなり、
			// 画面のコスト指標切り替えが壊れる。
			wantMetrics := []string{"UnblendedCost", "NetAmortizedCost"}
			if diff := cmp.Diff(wantMetrics, fake.gotInput.Metrics); diff != "" {
				t.Errorf("Metrics mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetCostTimePeriod(t *testing.T) {
	fake := &fakeCostExplorer{out: &costexplorer.GetCostAndUsageOutput{}}
	opts := CostQueryOptions{StartDate: "2026-06-01", EndDate: "2026-06-30", Granularity: "MONTHLY"}
	if _, err := getCost(context.Background(), fake, opts); err != nil {
		t.Fatalf("getCost() error = %v, want nil", err)
	}
	if fake.gotInput.TimePeriod == nil {
		t.Fatal("TimePeriod is nil")
	}
	if got := ptrStr(fake.gotInput.TimePeriod.Start); got != "2026-06-01" {
		t.Errorf("TimePeriod.Start = %q, want %q", got, "2026-06-01")
	}
	if got := ptrStr(fake.gotInput.TimePeriod.End); got != "2026-06-30" {
		t.Errorf("TimePeriod.End = %q, want %q", got, "2026-06-30")
	}
	if fake.gotInput.Granularity != cetypes.GranularityMonthly {
		t.Errorf("Granularity = %v, want %v", fake.gotInput.Granularity, cetypes.GranularityMonthly)
	}
}

func TestGetCostResults(t *testing.T) {
	out := &costexplorer.GetCostAndUsageOutput{
		ResultsByTime: []cetypes.ResultByTime{
			{
				TimePeriod: &cetypes.DateInterval{Start: aws.String("2026-07-01"), End: aws.String("2026-07-02")},
				Groups: []cetypes.Group{
					{
						Keys: []string{"AmazonEC2"},
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost":    {Amount: aws.String("10.5"), Unit: aws.String("USD")},
							"NetAmortizedCost": {Amount: aws.String("12.25"), Unit: aws.String("USD")},
						},
					},
					{
						// UnblendedCost が無い場合は Unit を NetAmortizedCost から拾う
						Keys: []string{"AmazonS3"},
						Metrics: map[string]cetypes.MetricValue{
							"NetAmortizedCost": {Amount: aws.String("1.5"), Unit: aws.String("USD")},
						},
					},
					{
						// パース不能な金額は 0 として扱い、Keys が空なら Service も空になる
						Keys: nil,
						Metrics: map[string]cetypes.MetricValue{
							"UnblendedCost": {Amount: aws.String("not-a-number"), Unit: aws.String("USD")},
						},
					},
				},
			},
			{
				// TimePeriod が nil の結果は期間を空文字として扱う
				Groups: []cetypes.Group{
					{Keys: []string{"AmazonEC2"}, Metrics: map[string]cetypes.MetricValue{}},
				},
			},
		},
	}
	want := []CostResource{
		{TimePeriod: "2026-07-01", Service: "AmazonEC2", UnblendedAmount: 10.5, NetAmortizedAmount: 12.25, Unit: "USD"},
		{TimePeriod: "2026-07-01", Service: "AmazonS3", UnblendedAmount: 0, NetAmortizedAmount: 1.5, Unit: "USD"},
		{TimePeriod: "2026-07-01", Service: "", UnblendedAmount: 0, NetAmortizedAmount: 0, Unit: "USD"},
		{TimePeriod: "", Service: "AmazonEC2", UnblendedAmount: 0, NetAmortizedAmount: 0, Unit: ""},
	}

	got, err := getCost(context.Background(), &fakeCostExplorer{out: out}, CostQueryOptions{})
	if err != nil {
		t.Fatalf("getCost() error = %v, want nil", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestGetCostError(t *testing.T) {
	wantErr := errors.New("access denied")
	_, err := getCost(context.Background(), &fakeCostExplorer{err: wantErr}, CostQueryOptions{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want wrapped %v", err, wantErr)
	}
}
