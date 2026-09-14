// Datadog の Metrics 表示: Datadog クエリ言語の文字列をそのまま入力させ、指定期間の
// 時系列グラフを描く。メトリクス名やタグを GUI で組み立てるクエリビルダーは持たない
// (issue 0170 の設計判断で見送り)。
//
// 外枠 (.main とツールバー) は DatadogView が持ち、ここは中身だけを返す
// (DatadogDashboardView と同じ組み立て方)。
import { useState } from 'react';
import { useDatadogMetricsQueries } from '../../api/queries';
import { TimeseriesChart } from '../../components/charts/TimeseriesChart';
import { DatadogAuthBanner } from '../../components/DatadogAuthBanner';
import { ErrorBanner } from '../../components/ErrorBanner';
import { Icons } from '../../components/icons/Icons';
import { isDatadogAuthError } from '../../lib/datadogAuthError';
import { DEFAULT_METRICS_SPAN_SECONDS, metricsWindow, unitFormatter } from '../../lib/timeseries';

export interface DatadogMetricsViewProps {
  // 表示対象の組織 (小文字の public_id)。クエリの実行先と再ログインの対象を決める。
  orgId: string;
  // Dashboards のウィジェットから引き継いだクエリ。渡された場合は実行済みの状態で開く。
  initialQuery?: string;
}

// PERIOD_OPTIONS は遡る長さの選択肢。Datadog の既定のロールアップは窓の長さで決まるため、
// 粒度は指定させず窓の長さだけを選ばせる。
const PERIOD_OPTIONS: { label: string; seconds: number }[] = [
  { label: 'Last 1 hour', seconds: DEFAULT_METRICS_SPAN_SECONDS },
  { label: 'Last 4 hours', seconds: 4 * 3600 },
  { label: 'Last 1 day', seconds: 24 * 3600 },
  { label: 'Last 1 week', seconds: 7 * 24 * 3600 },
];

export function DatadogMetricsView({ orgId, initialQuery = '' }: DatadogMetricsViewProps) {
  const [queryInput, setQueryInput] = useState(initialQuery);
  // 入力の 1 文字ごとに Datadog へ問い合わせると、打ち終わる前の不正なクエリが
  // 何度もエラーになる。実行するクエリは確定操作 (Run または Enter) でのみ更新する。
  const [runningQuery, setRunningQuery] = useState(initialQuery.trim());
  const [spanSeconds, setSpanSeconds] = useState(DEFAULT_METRICS_SPAN_SECONDS);

  // 分に丸めた時間窓を毎描画で求める。同じ 1 分の間は同じ値になるのでクエリキーが
  // 安定し、時間の経過とともに窓が進む。
  const range = metricsWindow(spanSeconds);

  // 単一クエリの画面だが、複数クエリ版のフックへ長さ 1 の配列を渡して再利用する
  // (未入力の間は空配列を渡し、取得そのものを起こさない)。
  const { series, isLoading, error } = useDatadogMetricsQueries(
    orgId,
    runningQuery ? [runningQuery] : [],
    range,
  );

  const run = () => setRunningQuery(queryInput.trim());

  return (
    <>
      <div className="facets">
        <span className="chip-search">
          <Icons.search size={12} />
          <input
            aria-label="Query"
            value={queryInput}
            onChange={(e) => setQueryInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') run();
            }}
            placeholder="avg:system.cpu.user{*}"
            style={{ minWidth: 360 }}
          />
        </span>

        <select
          className="btn sm"
          aria-label="Period"
          value={spanSeconds}
          onChange={(e) => setSpanSeconds(Number(e.target.value))}
        >
          {PERIOD_OPTIONS.map((p) => (
            <option key={p.seconds} value={p.seconds}>
              {p.label}
            </option>
          ))}
        </select>

        <button className="btn sm" onClick={run} disabled={!queryInput.trim()}>
          Run
        </button>
      </div>

      {error &&
        (isDatadogAuthError(error) ? (
          <DatadogAuthBanner org={orgId} />
        ) : (
          <ErrorBanner error={error} />
        ))}

      {!runningQuery && (
        <div className="empty-hint">Enter a metric query to see its timeseries</div>
      )}

      {runningQuery && !error && isLoading && <div className="empty-hint">Loading…</div>}

      {runningQuery && !error && !isLoading && (
        <div className="stats" style={{ gridTemplateColumns: '1fr' }}>
          <div className="stat">
            <div className="label">{runningQuery}</div>
            <TimeseriesChart
              series={series.map((s) => ({ name: s.name, points: s.points }))}
              valueFormatter={unitFormatter(series[0]?.unit ?? '')}
            />
          </div>
        </div>
      )}
    </>
  );
}
