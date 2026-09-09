package aws

import (
	"context"
	"errors"
	"sync"
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

// dimensionPage は fakeCostExplorer が GetDimensionValues の応答を引くキー。
// 次元と、要求で受け取った NextPageToken (1 ページ目は空文字) の組で 1 ページを表す。
type dimensionPage struct {
	dim   cetypes.Dimension
	token string
}

// fakeCostExplorer は costExplorerAPI の手書きフェイク。受け取った入力を記録し、
// あらかじめ設定した出力またはエラーを返す。
// GetDimensionValues は次元ごとの goroutine から同時に呼ばれるため、記録は mu で保護する。
type fakeCostExplorer struct {
	gotInput *costexplorer.GetCostAndUsageInput
	out      *costexplorer.GetCostAndUsageOutput
	err      error

	// dimPages は GetDimensionValues の応答。キーに無いページを要求されたら空の応答を返す。
	dimPages map[dimensionPage]*costexplorer.GetDimensionValuesOutput
	// dimErrs は次元ごとに注入するエラー。特定の次元だけを失敗させるために使う。
	dimErrs map[cetypes.Dimension]error

	mu sync.Mutex
	// gotDimInputs は GetDimensionValues が受け取った入力を次元ごとに呼び出し順で保持する。
	gotDimInputs map[cetypes.Dimension][]*costexplorer.GetDimensionValuesInput
}

func (f *fakeCostExplorer) GetCostAndUsage(_ context.Context, p *costexplorer.GetCostAndUsageInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	f.gotInput = p
	if f.err != nil {
		return nil, f.err
	}
	return f.out, nil
}

func (f *fakeCostExplorer) GetDimensionValues(_ context.Context, p *costexplorer.GetDimensionValuesInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error) {
	f.mu.Lock()
	if f.gotDimInputs == nil {
		f.gotDimInputs = make(map[cetypes.Dimension][]*costexplorer.GetDimensionValuesInput)
	}
	f.gotDimInputs[p.Dimension] = append(f.gotDimInputs[p.Dimension], p)
	f.mu.Unlock()

	if err := f.dimErrs[p.Dimension]; err != nil {
		return nil, err
	}
	out, ok := f.dimPages[dimensionPage{dim: p.Dimension, token: ptrStr(p.NextPageToken)}]
	if !ok {
		return &costexplorer.GetDimensionValuesOutput{}, nil
	}
	return out, nil
}

// dimensionInputs は記録した GetDimensionValues の入力を、指定した次元について呼び出し順で返す。
func (f *fakeCostExplorer) dimensionInputs(dim cetypes.Dimension) []*costexplorer.GetDimensionValuesInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gotDimInputs[dim]
}

// dimensionCallCount は GetDimensionValues の総呼び出し回数を返す。
func (f *fakeCostExplorer) dimensionCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, inputs := range f.gotDimInputs {
		n += len(inputs)
	}
	return n
}

// plainValues は Attributes を持たない次元値の一覧を組み立てる。
func plainValues(values ...string) []cetypes.DimensionValuesWithAttributes {
	out := make([]cetypes.DimensionValuesWithAttributes, 0, len(values))
	for _, v := range values {
		out = append(out, cetypes.DimensionValuesWithAttributes{Value: aws.String(v)})
	}
	return out
}

// accountValue は LINKED_ACCOUNT の次元値 (Value がアカウント ID、Attributes の description が
// アカウント名) を組み立てる。属性名は実装の定数と独立に書き下し、定数の変更をテストが検出できる
// ようにする。Attributes の照合が LINKED_ACCOUNT に限られることを確認するケースでは、
// SERVICE / USAGE_TYPE の値に description 属性を付ける目的でも使う。
func accountValue(id, name string) cetypes.DimensionValuesWithAttributes {
	return cetypes.DimensionValuesWithAttributes{
		Value:      aws.String(id),
		Attributes: map[string]string{"description": name},
	}
}

// singlePage は次元ごとに 1 ページで完結する GetDimensionValues の応答を組み立てる。
func singlePage(byDim map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes) map[dimensionPage]*costexplorer.GetDimensionValuesOutput {
	pages := make(map[dimensionPage]*costexplorer.GetDimensionValuesOutput, len(byDim))
	for dim, values := range byDim {
		pages[dimensionPage{dim: dim}] = &costexplorer.GetDimensionValuesOutput{DimensionValues: values}
	}
	return pages
}

// dimensionExpr はテストの期待値として単一次元の EQUALS フィルタ式を組み立てる。
// 実装の costDimensionFilter とは独立に書き下すことで、実装の変更をテストが検出できるようにする。
func dimensionExpr(key cetypes.Dimension, values ...string) cetypes.Expression {
	return cetypes.Expression{
		Dimensions: &cetypes.DimensionValues{
			Key:          key,
			Values:       values,
			MatchOptions: []cetypes.MatchOption{cetypes.MatchOptionEquals},
		},
	}
}

// expressionCmpOpts は cetypes の構造体が持つ埋め込みの非公開シリアライズマーカーを比較対象から除く。
func expressionCmpOpts() cmp.Option {
	return cmpopts.IgnoreUnexported(cetypes.Expression{}, cetypes.DimensionValues{})
}

// テスト全体で使う次元値。testServiceEC2 と testServiceCompute は「ec2」というキーワードが
// 前者にだけ部分一致する組であり、大文字小文字を区別しない部分一致の検出に使う。
const (
	testServiceEC2     = "AmazonEC2"
	testServiceCompute = "Amazon Elastic Compute Cloud - Compute"
	testUsageType      = "APN1-BoxUsage:t3.medium"
	testUsageTypeEC2   = "APN1-EC2-Other"
	testAccountID      = "123456789012"
	testAccountName    = "prod-ec2-platform"
)

func TestGetCostFilter(t *testing.T) {
	serviceExpr := dimensionExpr(cetypes.DimensionService, testServiceEC2)
	accountExpr := dimensionExpr(cetypes.DimensionLinkedAccount, testAccountID)

	tests := []struct {
		name string
		opts CostQueryOptions
		// dimPages が nil のケースは GetDimensionValues が呼ばれないことを wantDimCalls=0 で固定する。
		dimPages     map[dimensionPage]*costexplorer.GetDimensionValuesOutput
		wantFilter   *cetypes.Expression
		wantGroupBy  string
		wantDimCalls int
	}{
		{
			name:         "Keyword 未指定の場合は GetDimensionValues を呼ばず Filter を設定しない",
			opts:         CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31"},
			wantFilter:   nil,
			wantGroupBy:  CostGroupByService,
			wantDimCalls: 0,
		},
		{
			// 空白のみの Keyword は TrimSpace 後に空になるため絞り込みなしとして扱う。
			// 空白を絞り込み条件として扱うと、全次元の値に部分一致して無意味な Or が組み上がる。
			name:         "空白のみの Keyword は絞り込みなしとして扱う",
			opts:         CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", Keyword: "   "},
			wantFilter:   nil,
			wantGroupBy:  CostGroupByService,
			wantDimCalls: 0,
		},
		{
			name: "SERVICE だけに一致した場合は SERVICE の単一 Dimensions がルートになる",
			opts: CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", Keyword: "AmazonEC2"},
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService:       plainValues(testServiceEC2, "AmazonS3"),
				cetypes.DimensionUsageType:     plainValues(testUsageType),
				cetypes.DimensionLinkedAccount: {accountValue(testAccountID, testAccountName)},
			}),
			wantFilter:   &serviceExpr,
			wantGroupBy:  CostGroupByService,
			wantDimCalls: 3,
		},
		{
			// 一致の無い次元 (USAGE_TYPE) が Or に含まれないことを固定する。含まれると
			// Values が空の Dimensions を AWS に送ることになる。
			name: "2 次元に一致した場合は Or に 2 要素を持ち一致の無い次元を含まない",
			opts: CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", Keyword: "ec2"},
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService:       plainValues(testServiceEC2, testServiceCompute),
				cetypes.DimensionUsageType:     plainValues("APN1-DataTransfer-Out-Bytes"),
				cetypes.DimensionLinkedAccount: {accountValue(testAccountID, testAccountName)},
			}),
			wantFilter: &cetypes.Expression{
				Or: []cetypes.Expression{
					serviceExpr,
					dimensionExpr(cetypes.DimensionLinkedAccount, testAccountID),
				},
			},
			wantGroupBy:  CostGroupByService,
			wantDimCalls: 3,
		},
		{
			name: "3 次元すべてに一致した場合は Or が 3 要素になる",
			opts: CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", Keyword: "ec2"},
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService:       plainValues(testServiceEC2),
				cetypes.DimensionUsageType:     plainValues(testUsageTypeEC2, "APN1-DataTransfer-Out-Bytes"),
				cetypes.DimensionLinkedAccount: {accountValue(testAccountID, testAccountName)},
			}),
			wantFilter: &cetypes.Expression{
				Or: []cetypes.Expression{
					serviceExpr,
					dimensionExpr(cetypes.DimensionUsageType, testUsageTypeEC2),
					accountExpr,
				},
			},
			wantGroupBy:  CostGroupByService,
			wantDimCalls: 3,
		},
		{
			// アカウント名 (Attributes の description) にだけ一致する場合、Values には
			// 名前ではなく Value (アカウント ID) が入る。Cost Explorer の LINKED_ACCOUNT の
			// Filter が受け付けるのはアカウント ID であるため。
			name: "アカウント名だけに一致した場合は Values にアカウント ID が入る",
			opts: CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", Keyword: "platform"},
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService:       plainValues(testServiceEC2),
				cetypes.DimensionUsageType:     plainValues(testUsageType),
				cetypes.DimensionLinkedAccount: {accountValue(testAccountID, testAccountName)},
			}),
			wantFilter:   &accountExpr,
			wantGroupBy:  CostGroupByService,
			wantDimCalls: 3,
		},
		{
			// Attributes が nil の LINKED_ACCOUNT 値でも panic せず、Value だけを照合する。
			// nil マップの索引が ok=false を返すことに依存した経路。
			name: "Attributes が nil の LINKED_ACCOUNT 値は Value だけを照合する",
			opts: CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", Keyword: "1234"},
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService:       plainValues(testServiceEC2),
				cetypes.DimensionUsageType:     plainValues(testUsageType),
				cetypes.DimensionLinkedAccount: plainValues(testAccountID),
			}),
			wantFilter:   &accountExpr,
			wantGroupBy:  CostGroupByService,
			wantDimCalls: 3,
		},
		{
			// Attributes の照合は LINKED_ACCOUNT に限る。SERVICE の値に description 属性が
			// 付いていてキーワードに一致しても、SERVICE の値としては一致扱いにしない。
			// 全次元で Attributes を照合する実装に変わるとこのケースだけが落ちる。
			name: "LINKED_ACCOUNT 以外の次元では Attributes を照合しない",
			opts: CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", Keyword: "platform"},
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService:       {accountValue(testServiceEC2, testAccountName)},
				cetypes.DimensionUsageType:     {accountValue(testUsageType, testAccountName)},
				cetypes.DimensionLinkedAccount: {accountValue(testAccountID, testAccountName)},
			}),
			wantFilter:   &accountExpr,
			wantGroupBy:  CostGroupByService,
			wantDimCalls: 3,
		},
		{
			// 照合が大文字小文字を区別しないこと、かつ部分一致であって別名解決ではないことを
			// 同時に固定する。「ec2」は AmazonEC2 に一致し、同じ EC2 を指す正式名称である
			// 「Amazon Elastic Compute Cloud - Compute」には一致しない。
			name: "照合は大文字小文字を区別しない部分一致で行う",
			opts: CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", Keyword: "eC2"},
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService: plainValues(testServiceCompute, testServiceEC2),
			}),
			wantFilter:   &serviceExpr,
			wantGroupBy:  CostGroupByService,
			wantDimCalls: 3,
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
				Keyword:          "AmazonEC2",
			},
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService: plainValues(testServiceEC2),
			}),
			wantFilter:   &serviceExpr,
			wantGroupBy:  CostGroupByService,
			wantDimCalls: 3,
		},
		{
			// GroupBy と Keyword は独立した概念であり、絞り込む次元と集計する次元は一致しなくてよい。
			// Usage type を表示したままサービス名で絞り込めることが本 issue の要望そのものである。
			name: "GroupBy=USAGE_TYPE のまま Keyword がサービス名に一致する",
			opts: CostQueryOptions{
				StartDate:        "2026-07-01",
				EndDate:          "2026-07-31",
				GroupByDimension: CostGroupByUsageType,
				Keyword:          "AmazonEC2",
			},
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService: plainValues(testServiceEC2),
			}),
			wantFilter:   &serviceExpr,
			wantGroupBy:  CostGroupByUsageType,
			wantDimCalls: 3,
		},
		{
			name: "GroupBy=REGION のまま Keyword がサービス名とアカウント ID の両方に一致する",
			opts: CostQueryOptions{
				StartDate:        "2026-07-01",
				EndDate:          "2026-07-31",
				GroupByDimension: CostGroupByRegion,
				Keyword:          "2",
			},
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService:       plainValues(testServiceEC2),
				cetypes.DimensionUsageType:     plainValues("APN1-DataTransfer-Out-Bytes"),
				cetypes.DimensionLinkedAccount: plainValues(testAccountID),
			}),
			wantFilter: &cetypes.Expression{
				Or: []cetypes.Expression{serviceExpr, accountExpr},
			},
			wantGroupBy:  CostGroupByRegion,
			wantDimCalls: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeCostExplorer{
				out:      &costexplorer.GetCostAndUsageOutput{},
				dimPages: tt.dimPages,
			}
			if _, err := getCost(context.Background(), fake, tt.opts); err != nil {
				t.Fatalf("getCost() error = %v, want nil", err)
			}
			if got := fake.dimensionCallCount(); got != tt.wantDimCalls {
				t.Errorf("GetDimensionValues call count = %d, want %d", got, tt.wantDimCalls)
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

func TestGetCostKeywordNoMatch(t *testing.T) {
	fake := &fakeCostExplorer{
		out: &costexplorer.GetCostAndUsageOutput{},
		dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
			cetypes.DimensionService:       plainValues(testServiceEC2),
			cetypes.DimensionUsageType:     plainValues(testUsageType),
			cetypes.DimensionLinkedAccount: {accountValue(testAccountID, testAccountName)},
		}),
	}
	opts := CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", Keyword: "no-such-value"}

	got, err := getCost(context.Background(), fake, opts)
	if err != nil {
		t.Fatalf("getCost() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("resources = %v, want empty", got)
	}
	// 結果が空になることが確定しているため、課金される GetCostAndUsage を呼ばない。
	if fake.gotInput != nil {
		t.Error("GetCostAndUsage was called, want not called")
	}
}

func TestGetCostKeywordPaging(t *testing.T) {
	const (
		start = "2026-07-01"
		end   = "2026-07-31"
		token = "page-2"
	)
	fake := &fakeCostExplorer{
		out: &costexplorer.GetCostAndUsageOutput{},
		dimPages: map[dimensionPage]*costexplorer.GetDimensionValuesOutput{
			{dim: cetypes.DimensionService}: {
				DimensionValues: plainValues("AmazonS3"),
				NextPageToken:   aws.String(token),
			},
			// 2 ページ目にだけ一致する値を置き、全ページが照合対象になることを固定する。
			{dim: cetypes.DimensionService, token: token}: {
				DimensionValues: plainValues(testServiceEC2),
			},
		},
	}
	opts := CostQueryOptions{StartDate: start, EndDate: end, Keyword: "AmazonEC2"}

	if _, err := getCost(context.Background(), fake, opts); err != nil {
		t.Fatalf("getCost() error = %v, want nil", err)
	}
	if fake.gotInput == nil {
		t.Fatal("GetCostAndUsage was not called")
	}
	wantFilter := dimensionExpr(cetypes.DimensionService, testServiceEC2)
	if diff := cmp.Diff(&wantFilter, fake.gotInput.Filter, expressionCmpOpts()); diff != "" {
		t.Errorf("Filter mismatch (-want +got):\n%s", diff)
	}

	// SERVICE への呼び出しが 2 ページ分、呼び出し順で記録されていることを確認する。
	inputs := fake.dimensionInputs(cetypes.DimensionService)
	if len(inputs) != 2 {
		t.Fatalf("SERVICE call count = %d, want 2", len(inputs))
	}
	if got := ptrStr(inputs[0].NextPageToken); got != "" {
		t.Errorf("1 ページ目の NextPageToken = %q, want empty", got)
	}
	if got := ptrStr(inputs[1].NextPageToken); got != token {
		t.Errorf("2 ページ目の NextPageToken = %q, want %q", got, token)
	}
	// GetDimensionValues の期間は GetCostAndUsage と同じ costDateRange の結果を使う。
	if inputs[0].TimePeriod == nil {
		t.Fatal("TimePeriod is nil")
	}
	if got := ptrStr(inputs[0].TimePeriod.Start); got != start {
		t.Errorf("TimePeriod.Start = %q, want %q", got, start)
	}
	if got := ptrStr(inputs[0].TimePeriod.End); got != end {
		t.Errorf("TimePeriod.End = %q, want %q", got, end)
	}
	// SortBy を指定すると NextPageToken が使えず全ページを辿れない。
	if len(inputs[0].SortBy) != 0 {
		t.Errorf("SortBy = %v, want empty", inputs[0].SortBy)
	}
}

func TestGetCostDimensionValuesError(t *testing.T) {
	wantErr := errors.New("access denied")
	fake := &fakeCostExplorer{
		out: &costexplorer.GetCostAndUsageOutput{},
		dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
			cetypes.DimensionService:   plainValues(testServiceEC2),
			cetypes.DimensionUsageType: plainValues(testUsageTypeEC2),
		}),
		// 3 次元のうち LINKED_ACCOUNT だけを失敗させる。部分的に取得できた次元だけで
		// 絞り込むと、一致の無い次元が「一致無し」なのか「取得失敗」なのか区別できなくなる。
		dimErrs: map[cetypes.Dimension]error{cetypes.DimensionLinkedAccount: wantErr},
	}
	opts := CostQueryOptions{StartDate: "2026-07-01", EndDate: "2026-07-31", Keyword: "ec2"}

	_, err := getCost(context.Background(), fake, opts)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want wrapped %v", err, wantErr)
	}
	if fake.gotInput != nil {
		t.Error("GetCostAndUsage was called, want not called")
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

// linkedAccountOutput は 1 期間に、指定した Keys を持つ Groups を並べた GetCostAndUsage の応答を
// 組み立てる。keys の要素が nil の Group は Keys 無し (Service が空文字になる) を表す。
// Metrics は置かない (金額の変換は TestGetCostResults が担う)。
func linkedAccountOutput(period string, keys ...[]string) *costexplorer.GetCostAndUsageOutput {
	groups := make([]cetypes.Group, 0, len(keys))
	for _, k := range keys {
		groups = append(groups, cetypes.Group{Keys: k})
	}
	return &costexplorer.GetCostAndUsageOutput{
		ResultsByTime: []cetypes.ResultByTime{
			{
				TimePeriod: &cetypes.DateInterval{Start: aws.String(period), End: aws.String(period)},
				Groups:     groups,
			},
		},
	}
}

func TestGetCostLinkedAccountNames(t *testing.T) {
	const (
		start          = "2026-07-01"
		end            = "2026-07-31"
		token          = "page-2"
		otherAccountID = "210987654321"
		otherName      = "dev-sandbox"
	)
	linkedOpts := CostQueryOptions{StartDate: start, EndDate: end, GroupByDimension: CostGroupByLinkedAccount}
	accessDenied := errors.New("access denied")
	accountPage := func(values ...cetypes.DimensionValuesWithAttributes) map[dimensionPage]*costexplorer.GetDimensionValuesOutput {
		return singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
			cetypes.DimensionLinkedAccount: values,
		})
	}

	tests := []struct {
		name     string
		opts     CostQueryOptions
		out      *costexplorer.GetCostAndUsageOutput
		dimPages map[dimensionPage]*costexplorer.GetDimensionValuesOutput
		dimErrs  map[cetypes.Dimension]error
		want     []CostResource
		wantErr  error
		// wantAccountCalls は LINKED_ACCOUNT に対する GetDimensionValues の呼び出し回数 (ページ数を含む)。
		wantAccountCalls int
	}{
		{
			name:     "ID に対応する名前がある場合は AccountName に description が入る",
			opts:     linkedOpts,
			out:      linkedAccountOutput(start, []string{testAccountID}),
			dimPages: accountPage(accountValue(testAccountID, testAccountName)),
			want: []CostResource{
				{TimePeriod: start, Service: testAccountID, AccountName: testAccountName},
			},
			wantAccountCalls: 1,
		},
		{
			name:     "GetDimensionValues の結果に無い ID は AccountName が空文字",
			opts:     linkedOpts,
			out:      linkedAccountOutput(start, []string{testAccountID}, []string{otherAccountID}),
			dimPages: accountPage(accountValue(testAccountID, testAccountName)),
			want: []CostResource{
				{TimePeriod: start, Service: testAccountID, AccountName: testAccountName},
				{TimePeriod: start, Service: otherAccountID, AccountName: ""},
			},
			wantAccountCalls: 1,
		},
		{
			name:     "Attributes が nil の値は panic せず AccountName が空文字",
			opts:     linkedOpts,
			out:      linkedAccountOutput(start, []string{testAccountID}),
			dimPages: accountPage(plainValues(testAccountID)...),
			want: []CostResource{
				{TimePeriod: start, Service: testAccountID, AccountName: ""},
			},
			wantAccountCalls: 1,
		},
		{
			name:     "description が空文字の値は AccountName が空文字",
			opts:     linkedOpts,
			out:      linkedAccountOutput(start, []string{testAccountID}),
			dimPages: accountPage(accountValue(testAccountID, "")),
			want: []CostResource{
				{TimePeriod: start, Service: testAccountID, AccountName: ""},
			},
			wantAccountCalls: 1,
		},
		{
			name: "NextPageToken で 2 ページに分かれた結果は 2 ページ目の名前も入る",
			opts: linkedOpts,
			out:  linkedAccountOutput(start, []string{testAccountID}, []string{otherAccountID}),
			dimPages: map[dimensionPage]*costexplorer.GetDimensionValuesOutput{
				{dim: cetypes.DimensionLinkedAccount}: {
					DimensionValues: []cetypes.DimensionValuesWithAttributes{accountValue(testAccountID, testAccountName)},
					NextPageToken:   aws.String(token),
				},
				{dim: cetypes.DimensionLinkedAccount, token: token}: {
					DimensionValues: []cetypes.DimensionValuesWithAttributes{accountValue(otherAccountID, otherName)},
				},
			},
			want: []CostResource{
				{TimePeriod: start, Service: testAccountID, AccountName: testAccountName},
				{TimePeriod: start, Service: otherAccountID, AccountName: otherName},
			},
			wantAccountCalls: 2,
		},
		{
			// Value が空文字の次元値を対応表に入れると、Keys 無しで Service が空文字の CostResource に
			// 空文字キーの名前が付いてしまう。
			name: "Value が空文字の次元値は対応表に入らず Keys 無しの CostResource は AccountName が空文字のまま",
			opts: linkedOpts,
			out:  linkedAccountOutput(start, []string{testAccountID}, nil),
			dimPages: accountPage(
				accountValue(testAccountID, testAccountName),
				accountValue("", "name-for-empty-id"),
			),
			want: []CostResource{
				{TimePeriod: start, Service: testAccountID, AccountName: testAccountName},
				{TimePeriod: start, Service: "", AccountName: ""},
			},
			wantAccountCalls: 1,
		},
		{
			// 同じ ID が重複する応答は想定しないが、実装の書き方で挙動が変わらないよう後勝ちで固定する。
			name: "同じ Value が 1 ページ目と 2 ページ目に異なる description で現れたら 2 ページ目が勝つ",
			opts: linkedOpts,
			out:  linkedAccountOutput(start, []string{testAccountID}),
			dimPages: map[dimensionPage]*costexplorer.GetDimensionValuesOutput{
				{dim: cetypes.DimensionLinkedAccount}: {
					DimensionValues: []cetypes.DimensionValuesWithAttributes{accountValue(testAccountID, "old-name")},
					NextPageToken:   aws.String(token),
				},
				{dim: cetypes.DimensionLinkedAccount, token: token}: {
					DimensionValues: []cetypes.DimensionValuesWithAttributes{accountValue(testAccountID, "new-name")},
				},
			},
			want: []CostResource{
				{TimePeriod: start, Service: testAccountID, AccountName: "new-name"},
			},
			wantAccountCalls: 2,
		},
		{
			// 名前が引けないときに ID だけで縮退させると、失敗が画面から見えなくなる。
			name:             "GetDimensionValues がエラーを返したら getCost はラップしたエラーを返し CostResource を返さない",
			opts:             linkedOpts,
			out:              linkedAccountOutput(start, []string{testAccountID}),
			dimErrs:          map[cetypes.Dimension]error{cetypes.DimensionLinkedAccount: accessDenied},
			wantErr:          accessDenied,
			wantAccountCalls: 1,
		},
		{
			// キーワード解決の取得結果は名前取得に再利用しないため、LINKED_ACCOUNT への呼び出しは
			// キーワード解決の 1 回と名前取得の 1 回の計 2 回になる。
			name: "キーワードが空でない場合は LINKED_ACCOUNT が 2 回呼ばれ AccountName が入る",
			opts: CostQueryOptions{StartDate: start, EndDate: end, GroupByDimension: CostGroupByLinkedAccount, Keyword: testAccountID},
			out:  linkedAccountOutput(start, []string{testAccountID}),
			dimPages: singlePage(map[cetypes.Dimension][]cetypes.DimensionValuesWithAttributes{
				cetypes.DimensionService:       plainValues(testServiceEC2),
				cetypes.DimensionUsageType:     plainValues(testUsageType),
				cetypes.DimensionLinkedAccount: {accountValue(testAccountID, testAccountName)},
			}),
			want: []CostResource{
				{TimePeriod: start, Service: testAccountID, AccountName: testAccountName},
			},
			wantAccountCalls: 2,
		},
		{
			name:     "GroupByDimension が空文字 (SERVICE) なら GetDimensionValues を呼ばず AccountName は空文字",
			opts:     CostQueryOptions{StartDate: start, EndDate: end},
			out:      linkedAccountOutput(start, []string{testAccountID}),
			dimPages: accountPage(accountValue(testAccountID, testAccountName)),
			want: []CostResource{
				{TimePeriod: start, Service: testAccountID, AccountName: ""},
			},
			wantAccountCalls: 0,
		},
		{
			name:     "GroupByDimension が USAGE_TYPE なら GetDimensionValues を呼ばず AccountName は空文字",
			opts:     CostQueryOptions{StartDate: start, EndDate: end, GroupByDimension: CostGroupByUsageType},
			out:      linkedAccountOutput(start, []string{testUsageType}),
			dimPages: accountPage(accountValue(testUsageType, testAccountName)),
			want: []CostResource{
				{TimePeriod: start, Service: testUsageType, AccountName: ""},
			},
			wantAccountCalls: 0,
		},
		{
			// 名前を引く相手がいないため、リクエストごとに課金される GetDimensionValues を呼ばない。
			name:             "LINKED_ACCOUNT でも Groups が 1 つも無ければ GetDimensionValues を呼ばない",
			opts:             linkedOpts,
			out:              linkedAccountOutput(start),
			dimPages:         accountPage(accountValue(testAccountID, testAccountName)),
			want:             nil,
			wantAccountCalls: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeCostExplorer{out: tt.out, dimPages: tt.dimPages, dimErrs: tt.dimErrs}

			got, err := getCost(context.Background(), fake, tt.opts)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want wrapped %v", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("resources = %v, want nil", got)
				}
			} else {
				if err != nil {
					t.Fatalf("getCost() error = %v, want nil", err)
				}
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("mismatch (-want +got):\n%s", diff)
				}
			}

			inputs := fake.dimensionInputs(cetypes.DimensionLinkedAccount)
			if len(inputs) != tt.wantAccountCalls {
				t.Fatalf("LINKED_ACCOUNT call count = %d, want %d", len(inputs), tt.wantAccountCalls)
			}
			// 名前取得の期間は GetCostAndUsage と同じ costDateRange の結果を使う。
			for i, in := range inputs {
				if in.TimePeriod == nil {
					t.Fatalf("inputs[%d].TimePeriod is nil", i)
				}
				if s, e := ptrStr(in.TimePeriod.Start), ptrStr(in.TimePeriod.End); s != start || e != end {
					t.Errorf("inputs[%d].TimePeriod = (%q, %q), want (%q, %q)", i, s, e, start, end)
				}
			}
		})
	}
}
