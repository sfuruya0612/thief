// cost 集計 (AWS / Datadog / TiDB) の共通コア。ドメインごとの違いはアクセサで注入する。
import type { CostCrossTableRow } from '../components/tables/CostCrossTable';
import type { CostChartSeries } from '../components/charts/CostChart';

export interface CostAggregateResult {
  categories: string[];
  series: CostChartSeries[];
  crossTableRows: CostCrossTableRow[];
  total: number;
}

// CostRowAccessors は行からカテゴリ軸 (日付/月)・グループキー・金額を取り出すアクセサ。
export interface CostRowAccessors<T> {
  categoryOf: (r: T) => string;
  groupKeyOf: (r: T) => string;
  amountOf: (r: T) => number;
}

// 積み上げグラフの系列が多すぎると凡例が読めなくなるため、金額の大きい上位のみ個別系列にし
// 残りは Other にまとめる (クロス表側は Other にまとめず全グループを表示する)。
export function aggregateCostRows<T>(
  rows: T[],
  acc: CostRowAccessors<T>,
  maxSeries: number,
): CostAggregateResult {
  const categorySet = new Set<string>();
  const totalsByGroup = new Map<string, number>();
  // series / クロス表用の (group, category) → 金額。同一キーの行が重複した場合は
  // set の上書きにより最後の行が勝つ (従来実装の new Map(entries) と同じ last-wins)。
  const byGroupCategory = new Map<string, Map<string, number>>();
  for (const r of rows) {
    const category = acc.categoryOf(r);
    const group = acc.groupKeyOf(r);
    const amount = acc.amountOf(r);
    categorySet.add(category);
    totalsByGroup.set(group, (totalsByGroup.get(group) ?? 0) + amount);
    let byCategory = byGroupCategory.get(group);
    if (!byCategory) {
      byCategory = new Map<string, number>();
      byGroupCategory.set(group, byCategory);
    }
    byCategory.set(category, amount);
  }
  const categories = [...categorySet].sort();

  const rankedGroups = [...totalsByGroup.entries()].sort((a, b) => b[1] - a[1]);
  const topGroups = rankedGroups.slice(0, maxSeries).map(([name]) => name);
  const hasOther = rankedGroups.length > maxSeries;

  const amountsFor = (group: string): number[] => {
    const byCategory = byGroupCategory.get(group);
    return categories.map((c) => byCategory?.get(c) ?? 0);
  };

  const series: CostChartSeries[] = topGroups.map((name) => ({
    name,
    data: amountsFor(name),
  }));
  if (hasOther) {
    const otherGroups = new Set(rankedGroups.slice(maxSeries).map(([name]) => name));
    const byCategory = new Map<string, number>();
    for (const r of rows) {
      if (!otherGroups.has(acc.groupKeyOf(r))) continue;
      const category = acc.categoryOf(r);
      byCategory.set(category, (byCategory.get(category) ?? 0) + acc.amountOf(r));
    }
    series.push({ name: 'Other', data: categories.map((c) => byCategory.get(c) ?? 0) });
  }

  // クロス表: 縦軸 = GroupBy の全グループ (Other にまとめない)、横軸 = カテゴリ (日付/月)
  const crossTableRows: CostCrossTableRow[] = rankedGroups.map(([group, groupTotal]) => ({
    group,
    amounts: amountsFor(group),
    total: groupTotal,
  }));

  const total = rows.reduce((sum, r) => sum + acc.amountOf(r), 0);
  return { categories, series, crossTableRows, total };
}
