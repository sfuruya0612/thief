// Cost Explorer のグラフ・クロス表向け集計ロジック。CostExplorerPanel から呼ばれる純関数群。
// 集計本体は lib/costAggregateCore.ts の aggregateCostRows に共通化されている。
import type { CostRow } from '../types/aws';
import { aggregateCostRows, type CostAggregateResult } from './costAggregateCore';

export type CostMetricType = 'unblended' | 'netAmortized';

export function amountOf(r: CostRow, metric: CostMetricType): number {
  return metric === 'unblended' ? r.unblendedAmount : r.netAmortizedAmount;
}

export type { CostAggregateResult };

export function aggregateCost(
  rows: CostRow[],
  metric: CostMetricType,
  maxSeries: number,
): CostAggregateResult {
  return aggregateCostRows(
    rows,
    {
      categoryOf: (r) => r.timePeriod,
      groupKeyOf: (r) => r.service,
      amountOf: (r) => amountOf(r, metric),
    },
    maxSeries,
  );
}
