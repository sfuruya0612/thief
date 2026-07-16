// 月次 (YYYY-MM) コスト表示の共通パネル。Total の stats、期間/グループ選択の facets、
// 積み上げグラフ + クロス集計表をまとめて描画する。Datadog / TiDB の Cost タブから使う。
// 期間・グループ選択・フィルタの state はタブ切替で消えないよう親 (ビュー) 側が保持する。
import { useMemo } from 'react';
import { CostChart } from './charts/CostChart';
import { CostCrossTable } from './tables/CostCrossTable';
import { Loading } from './Loading';
import { Icons } from './icons/Icons';
import { MONTH_RANGE_PRESETS } from '../lib/monthRange';
import type { CostAggregateResult } from '../lib/costAggregateCore';

// 積み上げグラフの系列が多すぎると凡例が読めなくなるため、金額の大きい上位のみ個別系列にし
// 残りは Other にまとめる
const MAX_SERIES = 8;

export interface MonthlyCostPanelProps<R, G extends string> {
  rows: R[];
  isLoading: boolean;
  groupByOptions: { value: G; label: string }[];
  groupBy: G;
  onGroupByChange: (value: G) => void;
  groupFilter: string;
  onGroupFilterChange: (value: string) => void;
  startMonth: string;
  endMonth: string;
  onStartMonthChange: (value: string) => void;
  onEndMonthChange: (value: string) => void;
  onApplyPreset: (months: number) => void;
  aggregate: (rows: R[], groupBy: G, maxSeries: number) => CostAggregateResult;
  // groupValueOf はフィルタ対象のグループ名を行から取り出す。useMemo の依存に入るため
  // モジュールスコープの安定した関数を渡すこと。
  groupValueOf: (row: R, groupBy: G) => string;
}

export function MonthlyCostPanel<R, G extends string>({
  rows,
  isLoading,
  groupByOptions,
  groupBy,
  onGroupByChange,
  groupFilter,
  onGroupFilterChange,
  startMonth,
  endMonth,
  onStartMonthChange,
  onEndMonthChange,
  onApplyPreset,
  aggregate,
  groupValueOf,
}: MonthlyCostPanelProps<R, G>) {
  // グループ名フィルタは取得済みデータに対してブラウザ側で絞り込むだけにし、都度 API を
  // 呼び出さない (AWS Cost Explorer と同じ方式)。
  const filteredRows = useMemo(() => {
    if (!groupFilter.trim()) return rows;
    const needle = groupFilter.trim().toLowerCase();
    return rows.filter((r) => groupValueOf(r, groupBy).toLowerCase().includes(needle));
  }, [rows, groupFilter, groupBy, groupValueOf]);

  const { categories, series, crossTableRows, total } = useMemo(
    () => aggregate(filteredRows, groupBy, MAX_SERIES),
    [aggregate, filteredRows, groupBy],
  );

  return (
    <>
      <div className="stats" style={{ gridTemplateColumns: 'repeat(1, 1fr)' }}>
        <div className="stat">
          <div className="label">Total</div>
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
            value={groupFilter}
            onChange={(e) => onGroupFilterChange(e.target.value)}
            placeholder={`filter by ${groupByOptions.find((o) => o.value === groupBy)?.label.toLowerCase()} name (client-side)…`}
          />
        </span>

        <input
          type="month"
          className="btn sm"
          value={startMonth}
          max={endMonth}
          onChange={(e) => onStartMonthChange(e.target.value)}
          title="Start month"
        />
        <span style={{ color: 'var(--text-3)' }}>–</span>
        <input
          type="month"
          className="btn sm"
          value={endMonth}
          min={startMonth}
          onChange={(e) => onEndMonthChange(e.target.value)}
          title="End month"
        />

        {MONTH_RANGE_PRESETS.map((p) => (
          <button
            key={p.label}
            className="btn sm ghost"
            onClick={() => onApplyPreset(p.months)}
            title={p.label}
          >
            {p.label}
          </button>
        ))}

        <select
          className="btn sm"
          value={groupBy}
          onChange={(e) => onGroupByChange(e.target.value as G)}
          title="Group by"
        >
          {groupByOptions.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      </div>

      {isLoading ? (
        <Loading />
      ) : (
        <>
          <CostChart categories={categories} series={series} />
          <CostCrossTable categories={categories} rows={crossTableRows} />
        </>
      )}
    </>
  );
}
