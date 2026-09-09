import { describe, expect, it } from 'vitest';
import { aggregateCost, costGroupLabel } from './costAggregate';
import type { CostRow } from '../types/aws';

// accountName は Group by が Linked account のときだけ backend が返すため、既定は空文字にする。
function row(
  timePeriod: string,
  service: string,
  unblendedAmount: number,
  netAmortizedAmount: number,
  accountName = '',
): CostRow {
  return {
    id: `${timePeriod}/${service}`,
    timePeriod,
    service,
    accountName,
    unblendedAmount,
    netAmortizedAmount,
    unit: 'USD',
  };
}

describe('aggregateCost', () => {
  const rows: CostRow[] = [
    row('2026-07-01', 'AmazonEC2', 10, 12),
    row('2026-07-01', 'AmazonS3', 1, 2),
    row('2026-07-02', 'AmazonEC2', 20, 22),
    row('2026-07-02', 'AmazonS3', 2, 3),
  ];

  it('categories を日付昇順で返す', () => {
    const { categories } = aggregateCost(rows, 'unblended', 8);
    expect(categories).toEqual(['2026-07-01', '2026-07-02']);
  });

  it('unblended 指定時は unblendedAmount を集計する', () => {
    const { total, crossTableRows } = aggregateCost(rows, 'unblended', 8);
    expect(total).toBe(33); // 10 + 1 + 20 + 2
    const ec2 = crossTableRows.find((r) => r.group === 'AmazonEC2');
    expect(ec2).toEqual({ group: 'AmazonEC2', amounts: [10, 20], total: 30 });
  });

  it('netAmortized 指定時は netAmortizedAmount を集計する', () => {
    const { total, crossTableRows } = aggregateCost(rows, 'netAmortized', 8);
    expect(total).toBe(39); // 12 + 2 + 22 + 3
    const ec2 = crossTableRows.find((r) => r.group === 'AmazonEC2');
    expect(ec2).toEqual({ group: 'AmazonEC2', amounts: [12, 22], total: 34 });
  });

  it('crossTableRows は金額降順で全グループを含む (Other にまとめない)', () => {
    const { crossTableRows } = aggregateCost(rows, 'unblended', 1);
    expect(crossTableRows.map((r) => r.group)).toEqual(['AmazonEC2', 'AmazonS3']);
  });

  it('series は maxSeries を超えるグループを Other にまとめる', () => {
    const { series } = aggregateCost(rows, 'unblended', 1);
    expect(series.map((s) => s.name)).toEqual(['AmazonEC2', 'Other']);
    const other = series.find((s) => s.name === 'Other');
    expect(other?.data).toEqual([1, 2]);
  });
});

describe('costGroupLabel', () => {
  it('accountName が空文字なら service をそのまま返す', () => {
    expect(costGroupLabel(row('2026-07-01', 'AmazonEC2', 1, 1))).toBe('AmazonEC2');
    expect(costGroupLabel(row('2026-07-01', '123456789012', 1, 1))).toBe('123456789012');
  });

  it('accountName が service と同じ値なら service だけを返す', () => {
    expect(costGroupLabel(row('2026-07-01', '123456789012', 1, 1, '123456789012'))).toBe(
      '123456789012',
    );
  });

  it('accountName が空でなく service と異なるなら "accountName (service)" を返す', () => {
    expect(costGroupLabel(row('2026-07-01', '123456789012', 1, 1, 'prod-platform'))).toBe(
      'prod-platform (123456789012)',
    );
  });
});

describe('aggregateCost (Linked account のラベル)', () => {
  const rows: CostRow[] = [
    row('2026-07-01', '123456789012', 10, 12, 'prod-platform'),
    row('2026-07-01', '210987654321', 1, 2), // 名前が登録されていないアカウント
    row('2026-07-02', '123456789012', 20, 22, 'prod-platform'),
    row('2026-07-02', '210987654321', 2, 3),
  ];

  it('crossTableRows の group は accountName を持つ行では "Account Name (Account ID)"、無い行では service のまま', () => {
    const { crossTableRows } = aggregateCost(rows, 'unblended', 8);
    expect(crossTableRows).toEqual([
      { group: 'prod-platform (123456789012)', amounts: [10, 20], total: 30 },
      { group: '210987654321', amounts: [1, 2], total: 3 },
    ]);
  });

  it('series の name も crossTableRows の group と同じラベルになる', () => {
    const { series } = aggregateCost(rows, 'unblended', 8);
    expect(series.map((s) => s.name)).toEqual(['prod-platform (123456789012)', '210987654321']);
  });

  it('同じ accountName で service (ID) が異なる 2 行は別の群として集計される', () => {
    const sameName: CostRow[] = [
      row('2026-07-01', '111111111111', 5, 5, 'shared-name'),
      row('2026-07-01', '222222222222', 7, 7, 'shared-name'),
    ];
    const { crossTableRows, series } = aggregateCost(sameName, 'unblended', 8);
    expect(crossTableRows.map((r) => r.group)).toEqual([
      'shared-name (222222222222)',
      'shared-name (111111111111)',
    ]);
    expect(series.map((s) => s.name)).toEqual([
      'shared-name (222222222222)',
      'shared-name (111111111111)',
    ]);
  });
});
