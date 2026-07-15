import { describe, expect, it } from 'vitest';
import { aggregateDatadogCost } from './costAggregateDatadog';
import type { DatadogCostRow } from '../types/nonaws';

function row(
  month: string,
  productName: string,
  chargeType: string,
  accountName: string,
  orgName: string,
  cost: number,
): DatadogCostRow {
  return {
    id: `${month}-${productName}-${chargeType}`,
    month,
    accountName,
    orgName,
    productName,
    chargeType,
    cost,
  };
}

describe('aggregateDatadogCost', () => {
  const rows: DatadogCostRow[] = [
    row('2026-05', 'infra_host', 'on-demand', 'acct1', 'org1', 10),
    row('2026-05', 'logs', 'on-demand', 'acct1', 'org1', 1),
    row('2026-06', 'infra_host', 'on-demand', 'acct1', 'org1', 20),
    row('2026-06', 'logs', 'on-demand', 'acct1', 'org1', 2),
  ];

  it('categories を month 昇順で返す', () => {
    const { categories } = aggregateDatadogCost(rows, 'productName', 8);
    expect(categories).toEqual(['2026-05', '2026-06']);
  });

  it('groupBy=productName で cost を集計する', () => {
    const { total, crossTableRows } = aggregateDatadogCost(rows, 'productName', 8);
    expect(total).toBe(33); // 10 + 1 + 20 + 2
    const infra = crossTableRows.find((r) => r.group === 'infra_host');
    expect(infra).toEqual({ group: 'infra_host', amounts: [10, 20], total: 30 });
  });

  it('crossTableRows は金額降順で全グループを含む (Other にまとめない)', () => {
    const { crossTableRows } = aggregateDatadogCost(rows, 'productName', 1);
    expect(crossTableRows.map((r) => r.group)).toEqual(['infra_host', 'logs']);
  });

  it('series は maxSeries を超えるグループを Other にまとめる', () => {
    const { series } = aggregateDatadogCost(rows, 'productName', 1);
    expect(series.map((s) => s.name)).toEqual(['infra_host', 'Other']);
    const other = series.find((s) => s.name === 'Other');
    expect(other?.data).toEqual([1, 2]);
  });

  it('groupBy=accountName で集計する', () => {
    const multiAccount: DatadogCostRow[] = [
      row('2026-05', 'infra_host', 'on-demand', 'acct1', 'org1', 5),
      row('2026-05', 'infra_host', 'on-demand', 'acct2', 'org1', 7),
    ];
    const { crossTableRows } = aggregateDatadogCost(multiAccount, 'accountName', 8);
    expect(crossTableRows.map((r) => r.group)).toEqual(['acct2', 'acct1']);
  });
});
