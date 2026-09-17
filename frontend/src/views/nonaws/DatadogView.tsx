// Datadog ビュー: cost (historical / estimated の切替表示)、dashboards、metrics の
// セクション切替。cost は AWS Cost Explorer と同じ chart + クロス集計表。
import { useMemo, useState } from 'react';
import { useDatadogEstimated, useDatadogHistorical } from '../../api/queries';
import { MonthlyCostPanel } from '../../components/MonthlyCostPanel';
import { DatadogAuthBanner } from '../../components/DatadogAuthBanner';
import { ErrorBanner } from '../../components/ErrorBanner';
import { aggregateDatadogCost, type DatadogCostGroupBy } from '../../lib/costAggregateDatadog';
import { isDatadogAuthError } from '../../lib/datadogAuthError';
import { defaultMonthRange, lastMonthsRange } from '../../lib/monthRange';
import { DEFAULT_METRICS_SPAN_SECONDS } from '../../lib/timeseries';
import type { DatadogCostRow } from '../../types/nonaws';
import { DatadogDashboardView } from './DatadogDashboardView';
import { DatadogMetricsView } from './DatadogMetricsView';

type Mode = 'historical' | 'estimated';

// Section は Datadog ビューの中で切り替える表示の単位。
type Section = 'cost' | 'dashboards' | 'metrics';

const GROUP_BY_OPTIONS: { value: DatadogCostGroupBy; label: string }[] = [
  { value: 'productName', label: 'Product' },
  { value: 'chargeType', label: 'Charge type' },
  { value: 'orgName', label: 'Org' },
  { value: 'accountName', label: 'Account' },
];

function groupValueOf(r: DatadogCostRow, groupBy: DatadogCostGroupBy): string {
  return r[groupBy];
}

export interface DatadogViewProps {
  // 表示対象の組織 (小文字の public_id)。コスト取得と再ログインの対象を決める。
  orgId: string;
}

// Datadog の cost API は月単位でしか取得できないため、AWS Cost Explorer の日付範囲では
// なく年月 (YYYY-MM) の範囲で期間を指定する。estimated は当月/前月のみ有効。
export function DatadogView({ orgId }: DatadogViewProps) {
  const [section, setSection] = useState<Section>('cost');
  const [mode, setMode] = useState<Mode>('historical');
  // Metrics の入力状態。DatadogMetricsView はセクション切り替えでアンマウントされるため、
  // ここで持たないと入力中のクエリと選んだ期間が失われる (issue 0180)。組織タブの切り替えでは
  // App.tsx の key により DatadogView ごと作り直されるので、組織をまたいだ持ち越しは起きない。
  const [metricsQueryInput, setMetricsQueryInput] = useState('');
  const [metricsRunningQuery, setMetricsRunningQuery] = useState('');
  const [metricsSpanSeconds, setMetricsSpanSeconds] = useState(DEFAULT_METRICS_SPAN_SECONDS);

  const initialRange = useMemo(defaultMonthRange, []);
  const [startMonth, setStartMonth] = useState(initialRange.start);
  const [endMonth, setEndMonth] = useState(initialRange.end);
  const [groupBy, setGroupBy] = useState<DatadogCostGroupBy>('productName');
  const [groupFilter, setGroupFilter] = useState('');

  const {
    data: historical,
    error: historicalError,
    isLoading: historicalLoading,
  } = useDatadogHistorical(orgId, startMonth, endMonth, undefined, { enabled: section === 'cost' });
  const {
    data: estimated,
    error: estimatedError,
    isLoading: estimatedLoading,
  } = useDatadogEstimated(orgId, startMonth, endMonth, undefined, { enabled: section === 'cost' });

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
          <span className="subtitle">{section}</span>
        </div>
        <div className="seg" style={{ width: 300 }}>
          <button className={section === 'cost' ? 'active' : ''} onClick={() => setSection('cost')}>
            Cost
          </button>
          <button
            className={section === 'dashboards' ? 'active' : ''}
            onClick={() => setSection('dashboards')}
          >
            Dashboards
          </button>
          <button
            className={section === 'metrics' ? 'active' : ''}
            onClick={() => setSection('metrics')}
          >
            Metrics
          </button>
        </div>
        {section === 'cost' && (
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
        )}
      </div>

      {section === 'dashboards' && (
        <DatadogDashboardView
          orgId={orgId}
          onOpenQuery={(query) => {
            // Metrics に入力中の値が残っていても、利用者が選んだウィジェットのクエリで上書きする。
            setMetricsQueryInput(query);
            setMetricsRunningQuery(query.trim());
            setSection('metrics');
          }}
        />
      )}

      {section === 'metrics' && (
        <DatadogMetricsView
          orgId={orgId}
          queryInput={metricsQueryInput}
          onQueryInputChange={setMetricsQueryInput}
          runningQuery={metricsRunningQuery}
          onRunningQueryChange={setMetricsRunningQuery}
          spanSeconds={metricsSpanSeconds}
          onSpanSecondsChange={setMetricsSpanSeconds}
        />
      )}

      {section === 'cost' && (
        <>
          {error &&
            (isDatadogAuthError(error) ? (
              <DatadogAuthBanner org={orgId} />
            ) : (
              <ErrorBanner error={error} />
            ))}

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
        </>
      )}
    </div>
  );
}
