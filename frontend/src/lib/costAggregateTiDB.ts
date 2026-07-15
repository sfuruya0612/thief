// TiDB Cost Explorer 相当の chart / クロス表向け集計ロジック。lib/costAggregate.ts (AWS 版) と
// 同型だが、TiDBCostRow の billedDate/servicePathName 等のフィールドに合わせて別実装とする。
import type { TiDBCostRow } from '../types/nonaws';
import type { CostCrossTableRow } from '../components/tables/CostCrossTable';
import type { CostChartSeries } from '../components/charts/CostChart';

export type TiDBCostGroupBy = 'servicePathName' | 'projectName' | 'clusterName';

export interface TiDBCostAggregateResult {
  categories: string[];
  series: CostChartSeries[];
  crossTableRows: CostCrossTableRow[];
  total: number;
}

function groupKeyOf(r: TiDBCostRow, groupBy: TiDBCostGroupBy): string {
  return r[groupBy] || '(unknown)';
}

// 積み上げグラフの系列が多すぎると凡例が読めなくなるため、金額の大きい上位のみ個別系列にし
// 残りは Other にまとめる (クロス表側は Other にまとめず全グループを表示する)。
export function aggregateTiDBCost(
  rows: TiDBCostRow[],
  groupBy: TiDBCostGroupBy,
  maxSeries: number,
): TiDBCostAggregateResult {
  const categorySet = new Set<string>();
  const totalsByGroup = new Map<string, number>();
  for (const r of rows) {
    categorySet.add(r.billedDate);
    const key = groupKeyOf(r, groupBy);
    totalsByGroup.set(key, (totalsByGroup.get(key) ?? 0) + r.totalCost);
  }
  const categories = [...categorySet].sort();

  const rankedGroups = [...totalsByGroup.entries()].sort((a, b) => b[1] - a[1]);
  const topGroups = rankedGroups.slice(0, maxSeries).map(([name]) => name);
  const hasOther = rankedGroups.length > maxSeries;

  const series: CostChartSeries[] = topGroups.map((name) => {
    const byPeriod = new Map(
      rows.filter((r) => groupKeyOf(r, groupBy) === name).map((r) => [r.billedDate, r.totalCost]),
    );
    return { name, data: categories.map((c) => byPeriod.get(c) ?? 0) };
  });
  if (hasOther) {
    const otherGroups = new Set(rankedGroups.slice(maxSeries).map(([name]) => name));
    const byPeriod = new Map<string, number>();
    for (const r of rows) {
      const key = groupKeyOf(r, groupBy);
      if (!otherGroups.has(key)) continue;
      byPeriod.set(r.billedDate, (byPeriod.get(r.billedDate) ?? 0) + r.totalCost);
    }
    series.push({ name: 'Other', data: categories.map((c) => byPeriod.get(c) ?? 0) });
  }

  // クロス表: 縦軸 = GroupBy の全グループ (Other にまとめない)、横軸 = 日付
  const crossTableRows: CostCrossTableRow[] = rankedGroups.map(([group, groupTotal]) => {
    const byPeriod = new Map(
      rows.filter((r) => groupKeyOf(r, groupBy) === group).map((r) => [r.billedDate, r.totalCost]),
    );
    return {
      group,
      amounts: categories.map((c) => byPeriod.get(c) ?? 0),
      total: groupTotal,
    };
  });

  const total = rows.reduce((sum, r) => sum + r.totalCost, 0);
  return { categories, series, crossTableRows, total };
}
