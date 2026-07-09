// Cost Explorer 専用パネル。ServicePanel (汎用 15 サービス共通) とは異なり、
// リソース一覧ではなくコストの chart + 明細テーブルを表示するため専用実装とする。
import { useMemo, useState } from 'react';
import { useCost } from '../api/queries';
import { CostChart } from '../components/charts/CostChart';
import { DataTable } from '../components/DataTable';
import { costColumns } from '../components/tables/columns';
import { Icons } from '../components/icons/Icons';
import { ApiError } from '../types/common';
import { SSOExpiredBanner } from '../components/SSOExpiredBanner';

export interface CostExplorerPanelProps {
  profile: string;
  region: string;
}

const GRANULARITY_OPTIONS = [
  { value: 'DAILY', label: 'Daily' },
  { value: 'MONTHLY', label: 'Monthly' },
];

const GROUP_BY_OPTIONS = [
  { value: 'SERVICE', label: 'Service' },
  { value: 'USAGE_TYPE', label: 'Usage type' },
  { value: 'LINKED_ACCOUNT', label: 'Linked account' },
  { value: 'REGION', label: 'Region' },
];

const MONTHS_OPTIONS = [
  { value: 1, label: '直近 1 ヶ月' },
  { value: 3, label: '直近 3 ヶ月' },
  { value: 6, label: '直近 6 ヶ月' },
];

// 積み上げグラフの系列が多すぎると凡例が読めなくなるため、金額の大きい上位のみ個別系列にし
// 残りは Other にまとめる
const MAX_SERIES = 8;

export function CostExplorerPanel({ profile, region }: CostExplorerPanelProps) {
  const [granularity, setGranularity] = useState('DAILY');
  const [groupBy, setGroupBy] = useState('SERVICE');
  const [months, setMonths] = useState(1);
  const [serviceFilter, setServiceFilter] = useState('');

  const { data, error } = useCost(profile, region, {
    granularity,
    groupBy,
    months,
    service: serviceFilter || undefined,
  });

  const ssoExpired = error instanceof ApiError && error.code === 'SSO_TOKEN_EXPIRED';
  const rows = useMemo(() => data ?? [], [data]);

  const { categories, series, total } = useMemo(() => {
    const categorySet = new Set<string>();
    const totalsByGroup = new Map<string, number>();
    for (const r of rows) {
      categorySet.add(r.timePeriod);
      totalsByGroup.set(r.service, (totalsByGroup.get(r.service) ?? 0) + r.unblendedAmount);
    }
    const cats = [...categorySet].sort();

    const rankedGroups = [...totalsByGroup.entries()].sort((a, b) => b[1] - a[1]);
    const topGroups = rankedGroups.slice(0, MAX_SERIES).map(([name]) => name);
    const hasOther = rankedGroups.length > MAX_SERIES;

    const seriesData = topGroups.map((name) => {
      const byPeriod = new Map(
        rows.filter((r) => r.service === name).map((r) => [r.timePeriod, r.unblendedAmount]),
      );
      return { name, data: cats.map((c) => byPeriod.get(c) ?? 0) };
    });
    if (hasOther) {
      const otherGroups = new Set(rankedGroups.slice(MAX_SERIES).map(([name]) => name));
      const byPeriod = new Map<string, number>();
      for (const r of rows) {
        if (!otherGroups.has(r.service)) continue;
        byPeriod.set(r.timePeriod, (byPeriod.get(r.timePeriod) ?? 0) + r.unblendedAmount);
      }
      seriesData.push({ name: 'Other', data: cats.map((c) => byPeriod.get(c) ?? 0) });
    }

    const totalAmount = rows.reduce((sum, r) => sum + r.unblendedAmount, 0);
    return { categories: cats, series: seriesData, total: totalAmount };
  }, [rows]);

  return (
    <div className="main">
      <div className="toolbar">
        <div className="title">
          <h1>Cost Explorer</h1>
          <span className="subtitle">cost &amp; usage</span>
        </div>
      </div>

      {ssoExpired && <SSOExpiredBanner profile={profile} />}

      <div className="stats" style={{ gridTemplateColumns: 'repeat(1, 1fr)' }}>
        <div className="stat">
          <div className="label">Total (Unblended)</div>
          <div className="value">
            ${total.toLocaleString(undefined, { maximumFractionDigits: 2 })}
          </div>
          <div className="delta"> </div>
        </div>
      </div>

      <div className="facets">
        <span className="chip-search">
          <Icons.search size={12} />
          <input
            value={serviceFilter}
            onChange={(e) => setServiceFilter(e.target.value)}
            placeholder="filter by service name (exact match)…"
          />
        </span>

        <select
          className="btn sm"
          value={granularity}
          onChange={(e) => setGranularity(e.target.value)}
          title="Granularity"
        >
          {GRANULARITY_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>

        <select
          className="btn sm"
          value={groupBy}
          onChange={(e) => setGroupBy(e.target.value)}
          title="Group by"
        >
          {GROUP_BY_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>

        <select
          className="btn sm"
          value={months}
          onChange={(e) => setMonths(Number(e.target.value))}
          title="Period"
        >
          {MONTHS_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>

      <CostChart categories={categories} series={series} />

      <DataTable rows={rows} columns={costColumns} onSelect={() => {}} selectedId={null} />
    </div>
  );
}
