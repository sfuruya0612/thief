import { describe, expect, it } from 'vitest';
import { aggregateCost } from './costAggregate';
import type { CostRow } from '../types/aws';

function row(
  timePeriod: string,
  service: string,
  unblendedAmount: number,
  netAmortizedAmount: number,
): CostRow {
  return {
    id: `${timePeriod}/${service}`,
    timePeriod,
    service,
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
