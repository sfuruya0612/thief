// Datadog Cost Explorer 相当の chart / クロス表向け集計ロジック。
// 集計本体は lib/costAggregateCore.ts の aggregateCostRows に共通化されている。
import type { DatadogCostRow } from '../types/nonaws';
import { aggregateCostRows, type CostAggregateResult } from './costAggregateCore';

export type DatadogCostGroupBy = 'productName' | 'chargeType' | 'orgName' | 'accountName';

export function aggregateDatadogCost(
  rows: DatadogCostRow[],
  groupBy: DatadogCostGroupBy,
  maxSeries: number,
): CostAggregateResult {
  return aggregateCostRows(
    rows,
    {
      categoryOf: (r) => r.month,
      groupKeyOf: (r) => r[groupBy] || '(unknown)',
      amountOf: (r) => r.cost,
    },
    maxSeries,
  );
}
