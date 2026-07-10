// Cost Explorer のグラフ・クロス表向け集計ロジック。CostExplorerPanel から呼ばれる純関数群。
import type { CostRow } from '../types/aws';
import type { CostCrossTableRow } from '../components/tables/CostCrossTable';
import type { CostChartSeries } from '../components/charts/CostChart';

export type CostMetricType = 'unblended' | 'netAmortized';

export function amountOf(r: CostRow, metric: CostMetricType): number {
  return metric === 'unblended' ? r.unblendedAmount : r.netAmortizedAmount;
}

export interface CostAggregateResult {
  categories: string[];
  series: CostChartSeries[];
  crossTableRows: CostCrossTableRow[];
  total: number;
}

// 積み上げグラフの系列が多すぎると凡例が読めなくなるため、金額の大きい上位のみ個別系列にし
// 残りは Other にまとめる (クロス表側は Other にまとめず全グループを表示する)。
export function aggregateCost(
  rows: CostRow[],
  metric: CostMetricType,
  maxSeries: number,
): CostAggregateResult {
  const categorySet = new Set<string>();
  const totalsByGroup = new Map<string, number>();
  for (const r of rows) {
    categorySet.add(r.timePeriod);
    totalsByGroup.set(r.service, (totalsByGroup.get(r.service) ?? 0) + amountOf(r, metric));
  }
  const categories = [...categorySet].sort();

  const rankedGroups = [...totalsByGroup.entries()].sort((a, b) => b[1] - a[1]);
  const topGroups = rankedGroups.slice(0, maxSeries).map(([name]) => name);
  const hasOther = rankedGroups.length > maxSeries;

  const series: CostChartSeries[] = topGroups.map((name) => {
    const byPeriod = new Map(
      rows.filter((r) => r.service === name).map((r) => [r.timePeriod, amountOf(r, metric)]),
    );
    return { name, data: categories.map((c) => byPeriod.get(c) ?? 0) };
  });
  if (hasOther) {
    const otherGroups = new Set(rankedGroups.slice(maxSeries).map(([name]) => name));
    const byPeriod = new Map<string, number>();
    for (const r of rows) {
      if (!otherGroups.has(r.service)) continue;
      byPeriod.set(r.timePeriod, (byPeriod.get(r.timePeriod) ?? 0) + amountOf(r, metric));
    }
    series.push({ name: 'Other', data: categories.map((c) => byPeriod.get(c) ?? 0) });
  }

  // クロス表: 縦軸 = GroupBy の全グループ (Other にまとめない)、横軸 = 日付
  const crossTableRows: CostCrossTableRow[] = rankedGroups.map(([group, groupTotal]) => {
    const byPeriod = new Map(
      rows.filter((r) => r.service === group).map((r) => [r.timePeriod, amountOf(r, metric)]),
    );
    return {
      group,
      amounts: categories.map((c) => byPeriod.get(c) ?? 0),
      total: groupTotal,
    };
  });

  const total = rows.reduce((sum, r) => sum + amountOf(r, metric), 0);
  return { categories, series, crossTableRows, total };
}
