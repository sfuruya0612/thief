package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	cetypes "github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// recordingCostExplorer は costExplorerAPI の手書きモック。受け取った Input を値としてコピーし、
// 呼び出し順に記録する。cost_test.go の fakeCostExplorer は Input のポインタを 1 つだけ保持する形で
// 呼び出し回数を確かめられないため、この用途には使わない。
// 値でコピーするのは、getCostDetails がページングで同一の Input を使い回して NextPageToken を
// 書き換えるためである。
type recordingCostExplorer struct {
	inputs []costexplorer.GetCostAndUsageInput
}

func (m *recordingCostExplorer) GetCostAndUsage(_ context.Context, params *costexplorer.GetCostAndUsageInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	m.inputs = append(m.inputs, *params)
	return &costexplorer.GetCostAndUsageOutput{}, nil
}

// GetDimensionValues は costExplorerAPI を満たすためだけに置く。ここで検証する 4 経路は
// キーワードによる次元値の解決を行わないため、呼ばれた場合はエラーを返して気付けるようにする。
func (m *recordingCostExplorer) GetDimensionValues(_ context.Context, _ *costexplorer.GetDimensionValuesInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error) {
	return nil, errors.New("GetDimensionValues is not expected on this path")
}

// costInputCmpOpts は cetypes の構造体が持つ埋め込みの非公開シリアライズマーカーを比較対象から除く。
func costInputCmpOpts() cmp.Option {
	return cmpopts.IgnoreUnexported(cetypes.GroupDefinition{})
}

// TestGetCostDetailsSendsGroupBy は 4 つの取得経路が、それぞれの集計軸で GetCostAndUsage を
// 呼ぶことを検証する。期待値のディメンション名は実装の呼び出しを通さず AWS API の文字列で
// 直接書き下し、経路とディメンション名の対応を固定する。
// GroupBy を落とすと 3 つの内訳表示が「グルーピングなしの期間合計」に静かに縮退するため、
// 期間合計の経路では GroupBy が空のままであることも併せて固定する。
// metric と granularity は経路ごとに値を変える。全経路で同じ値にすると、引数を無視して
// その値を固定で送る実装に変えてもテストが通ってしまう。metric は 4 経路とも別の値にし、
// 実装が既定値に使う UnblendedCost は期待値に選ばない。granularity は API が 3 値しか
// 持たないため 1 組だけ重複するが、どの値を固定で送る実装に変えても 2 経路以上が落ちる。
func TestGetCostDetailsSendsGroupBy(t *testing.T) {
	const (
		startDate = "2026-07-01"
		endDate   = "2026-08-01"
	)

	dimensionGroupBy := func(key string) []cetypes.GroupDefinition {
		return []cetypes.GroupDefinition{{Key: aws.String(key), Type: cetypes.GroupDefinitionTypeDimension}}
	}

	tests := []struct {
		name        string
		granularity cetypes.Granularity
		metric      CostMetric
		call        func(ctx context.Context, client costExplorerAPI, granularity cetypes.Granularity, metric CostMetric) ([]CostDetail, error)
		wantGroupBy []cetypes.GroupDefinition
	}{
		{
			name:        "by service",
			granularity: cetypes.GranularityMonthly,
			metric:      BlendedCost,
			call: func(ctx context.Context, client costExplorerAPI, granularity cetypes.Granularity, metric CostMetric) ([]CostDetail, error) {
				return getCostByService(ctx, client, startDate, endDate, granularity, metric)
			},
			wantGroupBy: dimensionGroupBy("SERVICE"),
		},
		{
			name:        "by account",
			granularity: cetypes.GranularityHourly,
			metric:      NetUnblendedCost,
			call: func(ctx context.Context, client costExplorerAPI, granularity cetypes.Granularity, metric CostMetric) ([]CostDetail, error) {
				return getCostByAccount(ctx, client, startDate, endDate, granularity, metric)
			},
			wantGroupBy: dimensionGroupBy("LINKED_ACCOUNT"),
		},
		{
			name:        "by usage type",
			granularity: cetypes.GranularityDaily,
			metric:      AmortizedCost,
			call: func(ctx context.Context, client costExplorerAPI, granularity cetypes.Granularity, metric CostMetric) ([]CostDetail, error) {
				return getCostByUsageType(ctx, client, startDate, endDate, granularity, metric)
			},
			wantGroupBy: dimensionGroupBy("USAGE_TYPE"),
		},
		{
			name:        "for period",
			granularity: cetypes.GranularityMonthly,
			metric:      NormalizedUsageAmount,
			call: func(ctx context.Context, client costExplorerAPI, granularity cetypes.Granularity, metric CostMetric) ([]CostDetail, error) {
				return getCostForPeriod(ctx, client, startDate, endDate, granularity, metric)
			},
			wantGroupBy: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &recordingCostExplorer{}
			if _, err := tt.call(context.Background(), client, tt.granularity, tt.metric); err != nil {
				t.Fatalf("call: %v", err)
			}
			if len(client.inputs) != 1 {
				t.Fatalf("GetCostAndUsage called %d times, want 1", len(client.inputs))
			}
			in := client.inputs[0]
			if diff := cmp.Diff(tt.wantGroupBy, in.GroupBy, costInputCmpOpts()); diff != "" {
				t.Errorf("GroupBy mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff([]string{string(tt.metric)}, in.Metrics); diff != "" {
				t.Errorf("Metrics mismatch (-want +got):\n%s", diff)
			}
			if in.Granularity != tt.granularity {
				t.Errorf("Granularity = %q, want %q", in.Granularity, tt.granularity)
			}
		})
	}
}
