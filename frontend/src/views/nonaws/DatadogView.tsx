// Datadog ビュー: historical / estimated cost の切替表示 (AWS Cost Explorer と同じ chart + クロス集計表)
import { useMemo, useState } from 'react';
import { useDatadogEstimated, useDatadogHistorical } from '../../api/queries';
import { MonthlyCostPanel } from '../../components/MonthlyCostPanel';
import { ErrorBanner } from '../../components/ErrorBanner';
import { aggregateDatadogCost, type DatadogCostGroupBy } from '../../lib/costAggregateDatadog';
import { defaultMonthRange, lastMonthsRange } from '../../lib/monthRange';
import type { DatadogCostRow } from '../../types/nonaws';

type Mode = 'historical' | 'estimated';

const GROUP_BY_OPTIONS: { value: DatadogCostGroupBy; label: string }[] = [
  { value: 'productName', label: 'Product' },
  { value: 'chargeType', label: 'Charge type' },
  { value: 'orgName', label: 'Org' },
  { value: 'accountName', label: 'Account' },
];

function groupValueOf(r: DatadogCostRow, groupBy: DatadogCostGroupBy): string {
  return r[groupBy];
}

// Datadog の cost API は月単位でしか取得できないため、AWS Cost Explorer の日付範囲では
// なく年月 (YYYY-MM) の範囲で期間を指定する。estimated は当月/前月のみ有効。
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

  const applyPreset = (months: number) => {
    const range = lastMonthsRange(months);
    setStartMonth(range.start);
    setEndMonth(range.end);
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

      <MonthlyCostPanel
        rows={allRows}
        isLoading={isLoading}
        groupByOptions={GROUP_BY_OPTIONS}
        groupBy={groupBy}
        onGroupByChange={(g) => {
          setGroupBy(g);
          setGroupFilter('');
        }}
        groupFilter={groupFilter}
        onGroupFilterChange={setGroupFilter}
        startMonth={startMonth}
        endMonth={endMonth}
        onStartMonthChange={setStartMonth}
        onEndMonthChange={setEndMonth}
        onApplyPreset={applyPreset}
        aggregate={aggregateDatadogCost}
        groupValueOf={groupValueOf}
      />
    </div>
  );
}
