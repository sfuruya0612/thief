// TiDB Cost Explorer 相当の chart / クロス表向け集計ロジック。
// 集計本体は lib/costAggregateCore.ts の aggregateCostRows に共通化されている。
import type { TiDBCostRow } from '../types/nonaws';
import { aggregateCostRows, type CostAggregateResult } from './costAggregateCore';

export type TiDBCostGroupBy = 'servicePathName' | 'projectName' | 'clusterName';

export function aggregateTiDBCost(
  rows: TiDBCostRow[],
  groupBy: TiDBCostGroupBy,
  maxSeries: number,
): CostAggregateResult {
  return aggregateCostRows(
    rows,
    {
      categoryOf: (r) => r.billedDate,
      groupKeyOf: (r) => r[groupBy] || '(unknown)',
      amountOf: (r) => r.totalCost,
    },
    maxSeries,
  );
}
