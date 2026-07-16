import { describe, expect, it } from 'vitest';
import { aggregateCostRows, type CostRowAccessors } from './costAggregateCore';

interface Row {
  period: string;
  group: string;
  amount: number;
}

const acc: CostRowAccessors<Row> = {
  categoryOf: (r) => r.period,
  groupKeyOf: (r) => r.group,
  amountOf: (r) => r.amount,
};

describe('aggregateCostRows', () => {
  it('空入力では空の結果を返す', () => {
    const result = aggregateCostRows<Row>([], acc, 8);
    expect(result.categories).toEqual([]);
    expect(result.series).toEqual([]);
    expect(result.crossTableRows).toEqual([]);
    expect(result.total).toBe(0);
  });

  it('同一 (group, category) の重複行は series/クロス表では最後の行が勝ち、total には加算される', () => {
    const rows: Row[] = [
      { period: '2026-01', group: 'a', amount: 10 },
      { period: '2026-01', group: 'a', amount: 3 },
    ];
    const result = aggregateCostRows(rows, acc, 8);
    // 従来実装 (new Map(rows.filter().map())) の last-wins 挙動を維持する回帰テスト
    expect(result.series[0].data).toEqual([3]);
    expect(result.crossTableRows[0].amounts).toEqual([3]);
    // グループ合計と全体合計は全行の加算
    expect(result.crossTableRows[0].total).toBe(13);
    expect(result.total).toBe(13);
  });

  it('maxSeries を超えるグループは Other に集約されクロス表には全グループが残る', () => {
    const rows: Row[] = [
      { period: '2026-01', group: 'big', amount: 100 },
      { period: '2026-01', group: 'mid', amount: 50 },
      { period: '2026-01', group: 'small', amount: 1 },
    ];
    const result = aggregateCostRows(rows, acc, 2);
    expect(result.series.map((s) => s.name)).toEqual(['big', 'mid', 'Other']);
    expect(result.series[2].data).toEqual([1]);
    expect(result.crossTableRows.map((r) => r.group)).toEqual(['big', 'mid', 'small']);
  });

  it('カテゴリは昇順に整列されグループは合計金額の降順に並ぶ', () => {
    const rows: Row[] = [
      { period: '2026-02', group: 'a', amount: 1 },
      { period: '2026-01', group: 'b', amount: 5 },
    ];
    const result = aggregateCostRows(rows, acc, 8);
    expect(result.categories).toEqual(['2026-01', '2026-02']);
    expect(result.series.map((s) => s.name)).toEqual(['b', 'a']);
    expect(result.series[0].data).toEqual([5, 0]);
    expect(result.series[1].data).toEqual([0, 1]);
  });
});
