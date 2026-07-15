import { describe, expect, it } from 'vitest';
import { aggregateTiDBCost } from './costAggregateTiDB';
import type { TiDBCostRow } from '../types/nonaws';

function row(
  billedDate: string,
  servicePathName: string,
  projectName: string,
  clusterName: string,
  totalCost: number,
): TiDBCostRow {
  return {
    id: `${billedDate}-${projectName}-${clusterName}`,
    billedDate,
    projectName,
    clusterName,
    servicePathName,
    credits: 0,
    discounts: 0,
    runningTotal: 0,
    totalCost,
  };
}

describe('aggregateTiDBCost', () => {
  const rows: TiDBCostRow[] = [
    row('2026-05-01', 'compute', 'proj1', 'cluster1', 10),
    row('2026-05-01', 'storage', 'proj1', 'cluster1', 1),
    row('2026-06-01', 'compute', 'proj1', 'cluster1', 20),
    row('2026-06-01', 'storage', 'proj1', 'cluster1', 2),
  ];

  it('categories を billedDate 昇順で返す', () => {
    const { categories } = aggregateTiDBCost(rows, 'servicePathName', 8);
    expect(categories).toEqual(['2026-05-01', '2026-06-01']);
  });

  it('groupBy=servicePathName で totalCost を集計する', () => {
    const { total, crossTableRows } = aggregateTiDBCost(rows, 'servicePathName', 8);
    expect(total).toBe(33); // 10 + 1 + 20 + 2
    const compute = crossTableRows.find((r) => r.group === 'compute');
    expect(compute).toEqual({ group: 'compute', amounts: [10, 20], total: 30 });
  });

  it('crossTableRows は金額降順で全グループを含む (Other にまとめない)', () => {
    const { crossTableRows } = aggregateTiDBCost(rows, 'servicePathName', 1);
    expect(crossTableRows.map((r) => r.group)).toEqual(['compute', 'storage']);
  });

  it('series は maxSeries を超えるグループを Other にまとめる', () => {
    const { series } = aggregateTiDBCost(rows, 'servicePathName', 1);
    expect(series.map((s) => s.name)).toEqual(['compute', 'Other']);
    const other = series.find((s) => s.name === 'Other');
    expect(other?.data).toEqual([1, 2]);
  });

  it('groupBy=projectName で集計する', () => {
    const multiProject: TiDBCostRow[] = [
      row('2026-05-01', 'compute', 'proj1', 'cluster1', 5),
      row('2026-05-01', 'compute', 'proj2', 'cluster2', 7),
    ];
    const { crossTableRows } = aggregateTiDBCost(multiProject, 'projectName', 8);
    expect(crossTableRows.map((r) => r.group)).toEqual(['proj2', 'proj1']);
  });
});
