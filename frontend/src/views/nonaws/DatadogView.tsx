// Datadog ビュー: historical / estimated cost の切替表示 (AWS Cost Explorer と同じ chart + クロス集計表)
import { useMemo, useState } from 'react';
import { useDatadogEstimated, useDatadogHistorical } from '../../api/queries';
import { CostChart } from '../../components/charts/CostChart';
import { CostCrossTable } from '../../components/tables/CostCrossTable';
import { Loading } from '../../components/Loading';
import { Icons } from '../../components/icons/Icons';
import { ErrorBanner } from '../../components/ErrorBanner';
import { aggregateDatadogCost, type DatadogCostGroupBy } from '../../lib/costAggregateDatadog';

type Mode = 'historical' | 'estimated';

const GROUP_BY_OPTIONS: { value: DatadogCostGroupBy; label: string }[] = [
  { value: 'productName', label: 'Product' },
  { value: 'chargeType', label: 'Charge type' },
  { value: 'orgName', label: 'Org' },
  { value: 'accountName', label: 'Account' },
];

// Datadog の cost API は月単位でしか取得できないため、AWS Cost Explorer の日付範囲では
// なく年月 (YYYY-MM) の範囲で期間を指定する。estimated は当月/前月のみ有効。
const RANGE_PRESETS = [
  { label: '直近 3 ヶ月', months: 3 },
  { label: '直近 6 ヶ月', months: 6 },
  { label: '直近 12 ヶ月', months: 12 },
];

function toMonthInputValue(d: Date): string {
  return d.toISOString().slice(0, 7);
}

function defaultMonthRange(): { start: string; end: string } {
  const end = new Date();
  const start = new Date(end);
  start.setMonth(start.getMonth() - 2);
  return { start: toMonthInputValue(start), end: toMonthInputValue(end) };
}

// 積み上げグラフの系列が多すぎると凡例が読めなくなるため、金額の大きい上位のみ個別系列にし
// 残りは Other にまとめる
const MAX_SERIES = 8;

export function DatadogView() {
  const [mode, setMode] = useState<Mode>('historical');

  const initialRange = useMemo(defaultMonthRange, []);
  const [startMonth, setStartMonth] = useState(initialRange.start);
  const [endMonth, setEndMonth] = useState(initialRange.end);
  const [groupBy, setGroupBy] = useState<DatadogCostGroupBy>('productName');
  const [groupFilter, setGroupFilter] = useState('');

  const {
    data: historical,
    error: historicalError,
    isLoading: historicalLoading,
  } = useDatadogHistorical(startMonth, endMonth);
  const {
    data: estimated,
    error: estimatedError,
    isLoading: estimatedLoading,
  } = useDatadogEstimated(startMonth, endMonth);

  const error = mode === 'historical' ? historicalError : estimatedError;
  const isLoading = mode === 'historical' ? historicalLoading : estimatedLoading;
  const allRows = useMemo(
    () => (mode === 'historical' ? (historical ?? []) : (estimated ?? [])),
    [mode, historical, estimated],
  );

  // グループ名フィルタは取得済みデータに対してブラウザ側で絞り込むだけにし、都度 API を
  // 呼び出さない (AWS Cost Explorer と同じ方式)。
  const filteredRows = useMemo(() => {
    if (!groupFilter.trim()) return allRows;
    const needle = groupFilter.trim().toLowerCase();
    return allRows.filter((r) => r[groupBy].toLowerCase().includes(needle));
  }, [allRows, groupFilter, groupBy]);

  const { categories, series, crossTableRows, total } = useMemo(
    () => aggregateDatadogCost(filteredRows, groupBy, MAX_SERIES),
    [filteredRows, groupBy],
  );

  const applyPreset = (months: number) => {
    const end = new Date();
    const start = new Date(end);
    start.setMonth(start.getMonth() - (months - 1));
    setStartMonth(toMonthInputValue(start));
    setEndMonth(toMonthInputValue(end));
  };

  return (
    <div className="main">
      <div className="toolbar">
        <div className="title">
          <h1>Datadog</h1>
          <span className="subtitle">cost</span>
        </div>
        <div className="seg" style={{ width: 200 }}>
          <button
            className={mode === 'historical' ? 'active' : ''}
            onClick={() => setMode('historical')}
          >
            Historical
          </button>
          <button
            className={mode === 'estimated' ? 'active' : ''}
            onClick={() => setMode('estimated')}
          >
            Estimated
          </button>
        </div>
      </div>

      {error && <ErrorBanner error={error} />}

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
            onChange={(e) => setGroupFilter(e.target.value)}
            placeholder={`filter by ${GROUP_BY_OPTIONS.find((o) => o.value === groupBy)?.label.toLowerCase()} name (client-side)…`}
          />
        </span>

        <input
          type="month"
          className="btn sm"
          value={startMonth}
          max={endMonth}
          onChange={(e) => setStartMonth(e.target.value)}
          title="Start month"
        />
        <span style={{ color: 'var(--text-3)' }}>–</span>
        <input
          type="month"
          className="btn sm"
          value={endMonth}
          min={startMonth}
          onChange={(e) => setEndMonth(e.target.value)}
          title="End month"
        />

        {RANGE_PRESETS.map((p) => (
          <button
            key={p.label}
            className="btn sm ghost"
            onClick={() => applyPreset(p.months)}
            title={p.label}
          >
            {p.label}
          </button>
        ))}

        <select
          className="btn sm"
          value={groupBy}
          onChange={(e) => {
            setGroupBy(e.target.value as DatadogCostGroupBy);
            setGroupFilter('');
          }}
          title="Group by"
        >
          {GROUP_BY_OPTIONS.map((o) => (
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
    </div>
  );
}
