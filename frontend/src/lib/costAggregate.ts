// Cost Explorer のグラフ・クロス表向け集計ロジック。CostExplorerPanel から呼ばれる純関数群。
// 集計本体は lib/costAggregateCore.ts の aggregateCostRows に共通化されている。
import type { CostRow } from '../types/aws';
import { aggregateCostRows, type CostAggregateResult } from './costAggregateCore';

export type CostMetricType = 'unblended' | 'netAmortized';

export function amountOf(r: CostRow, metric: CostMetricType): number {
  return metric === 'unblended' ? r.unblendedAmount : r.netAmortizedAmount;
}

export type { CostAggregateResult };

// costGroupLabel はクロス表の Group 列とグラフの系列名に使う表示ラベルを返す。
// accountName は Group by が Linked account のときだけ backend が返すため、空文字なら service
// (サービス名 / 使用タイプ / アカウント ID / リージョン) をそのまま使い、groupBy の値では分岐しない。
// 名前と ID が同じときは ID だけを出す (Sidebar のリージョン表示と同じ扱い)。
// 同じ名前のアカウントが 2 つあってもラベルに ID が含まれるため別の群として集計される。
export function costGroupLabel(r: CostRow): string {
  if (r.accountName === '' || r.accountName === r.service) return r.service;
  return `${r.accountName} (${r.service})`;
}

export function aggregateCost(
  rows: CostRow[],
  metric: CostMetricType,
  maxSeries: number,
): CostAggregateResult {
  return aggregateCostRows(
    rows,
    {
      categoryOf: (r) => r.timePeriod,
      // ラベルを群キーにすることで、系列の順位付け、Other へのまとめ、クロス表の群キー、
      // グラフの系列名がすべて同じ文字列を使い、表示部品側での整形が要らない。
      groupKeyOf: costGroupLabel,
      amountOf: (r) => amountOf(r, metric),
    },
    maxSeries,
  );
}
