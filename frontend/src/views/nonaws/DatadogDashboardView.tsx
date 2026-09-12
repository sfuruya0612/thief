// Datadog の Dashboards 表示: 選択中の Sub Organization のダッシュボード一覧を出し、
// 選んだダッシュボードのウィジェットを thief 内で描く。
// ウィジェットの定義はクエリ文字列しか持たないため、値はクエリを実行して取得する
// (GET /api/datadog/metrics/query)。timeseries は TimeseriesChart、query_value は
// StatTile、それ以外の種別は Datadog へのリンクにフォールバックする
// (thief は全ウィジェット種別を再現しない)。
//
// 外枠 (.main とツールバー) は DatadogView が持ち、ここは中身だけを返す
// (MonthlyCostPanel と同じ組み立て方)。
import { useState } from 'react';
import {
  useDatadogDashboard,
  useDatadogDashboards,
  useDatadogMetricsQueries,
} from '../../api/queries';
import { StatTile } from '../../components/charts/StatTile';
import { TimeseriesChart } from '../../components/charts/TimeseriesChart';
import { DatadogAuthBanner } from '../../components/DatadogAuthBanner';
import { ErrorBanner } from '../../components/ErrorBanner';
import { isDatadogAuthError } from '../../lib/datadogAuthError';
import { metricsWindow, type MetricsWindow } from '../../lib/timeseries';
import type { DatadogMetricSeriesRow, DatadogWidgetRow } from '../../types/nonaws';

export interface DatadogDashboardViewProps {
  // 表示対象の組織 (小文字の public_id)。ダッシュボードの取得と再ログインの対象を決める。
  orgId: string;
}

// widgetLabel はウィジェットの見出し。タイトルが空のウィジェットは種別名で代替する。
function widgetLabel(widget: DatadogWidgetRow): string {
  return widget.title || widget.type;
}

// unitFormatter は単位付きの値の表示形式を返す。単位の無いメトリクスでは数値だけを出す。
function unitFormatter(unit: string): (v: number) => string {
  if (!unit) return (v: number) => v.toLocaleString();
  return (v: number) => `${v.toLocaleString()} ${unit}`;
}

// latestPoint は単一値表示に使う「最後に観測された値」と、その単位を返す。
//
// query_value は時間窓を 1 つの値に畳んだウィジェットなので、時系列の末尾の値を採る。
// 欠測を飛ばして遡るのは、窓の末端がまだ埋まっていないときに値が無いように見えるのを
// 防ぐため。
function latestPoint(series: DatadogMetricSeriesRow[]): { value: number | null; unit: string } {
  const first = series[0];
  if (!first) return { value: null, unit: '' };
  for (let i = first.points.length - 1; i >= 0; i--) {
    const v = first.points[i].v;
    if (v !== null) return { value: v, unit: first.unit };
  }
  return { value: null, unit: first.unit };
}

interface WidgetDataProps {
  orgId: string;
  widget: DatadogWidgetRow;
  range: MetricsWindow;
}

function TimeseriesWidget({ orgId, widget, range }: WidgetDataProps) {
  const { series, isLoading, error } = useDatadogMetricsQueries(orgId, widget.queries, range);

  return (
    <div className="stat">
      <div className="label">{widgetLabel(widget)}</div>
      {error && <div className="muted">{error.message}</div>}
      {!error && isLoading && <div className="empty-hint">Loading…</div>}
      {!error && !isLoading && (
        <TimeseriesChart
          series={series.map((s) => ({ name: s.name, points: s.points }))}
          valueFormatter={unitFormatter(series[0]?.unit ?? '')}
        />
      )}
    </div>
  );
}

function QueryValueWidget({ orgId, widget, range }: WidgetDataProps) {
  const { series, isLoading, error } = useDatadogMetricsQueries(orgId, widget.queries, range);

  if (error) {
    return (
      <div className="stat">
        <div className="label">{widgetLabel(widget)}</div>
        <div className="muted">{error.message}</div>
      </div>
    );
  }

  // 取得中に値を出すと、確定値なのか途中の値なのか区別が付かない。null (ダッシュ) のまま待つ。
  const { value, unit } = latestPoint(series);
  return (
    <StatTile
      title={widgetLabel(widget)}
      value={isLoading ? null : value}
      unit={unit || undefined}
    />
  );
}

interface WidgetCardProps extends WidgetDataProps {
  // ダッシュボードの絶対 URL (backend が app.<site> を補ったもの)。
  dashboardUrl: string;
}

function WidgetCard({ orgId, widget, range, dashboardUrl }: WidgetCardProps) {
  if (widget.kind === 'timeseries') {
    return <TimeseriesWidget orgId={orgId} widget={widget} range={range} />;
  }
  if (widget.kind === 'query_value') {
    return <QueryValueWidget orgId={orgId} widget={widget} range={range} />;
  }

  return (
    <div className="stat">
      <div className="label">{widgetLabel(widget)}</div>
      <div className="muted">{`thief does not render "${widget.type}" widgets.`}</div>
      {dashboardUrl && (
        <a className="btn sm" href={dashboardUrl} target="_blank" rel="noreferrer">
          Open in Datadog
        </a>
      )}
    </div>
  );
}

export function DatadogDashboardView({ orgId }: DatadogDashboardViewProps) {
  const [selectedId, setSelectedId] = useState('');

  const {
    data: dashboards,
    error: listError,
    isLoading: listLoading,
  } = useDatadogDashboards(orgId);
  const {
    data: detail,
    error: detailError,
    isLoading: detailLoading,
  } = useDatadogDashboard(orgId, selectedId);

  // 分に丸めた時間窓を毎描画で求める。同じ 1 分の間は同じ値になるのでウィジェットの
  // クエリキーが安定し、時間の経過とともに窓が進む。
  const range = metricsWindow();

  // 一覧と詳細のどちらの失敗も同じ場所に出す。一覧が失敗していれば詳細は取りに行かない
  // ため、両方が同時にエラーになることはない。
  const error = listError ?? detailError;
  const list = dashboards ?? [];

  return (
    <>
      <div className="facets">
        <select
          className="btn sm"
          aria-label="Dashboard"
          value={selectedId}
          onChange={(e) => setSelectedId(e.target.value)}
        >
          <option value="">Select a dashboard</option>
          {list.map((d) => (
            <option key={d.id} value={d.id}>
              {d.title}
            </option>
          ))}
        </select>
      </div>

      {error &&
        (isDatadogAuthError(error) ? (
          <DatadogAuthBanner org={orgId} />
        ) : (
          <ErrorBanner error={error} />
        ))}

      {(listLoading || detailLoading) && <div className="empty-hint">Loading…</div>}

      {!listLoading && !error && list.length === 0 && (
        <div className="empty-hint">No dashboards in this organization</div>
      )}

      {!selectedId && !listLoading && list.length > 0 && (
        <div className="empty-hint">Select a dashboard to see its widgets</div>
      )}

      {detail && (
        <div className="stats" style={{ gridTemplateColumns: '1fr' }}>
          {detail.widgets.length === 0 && (
            <div className="empty-hint">This dashboard has no widgets</div>
          )}
          {detail.widgets.map((w, i) => (
            <WidgetCard
              key={`${w.id}-${i}`}
              orgId={orgId}
              widget={w}
              range={range}
              dashboardUrl={detail.url}
            />
          ))}
        </div>
      )}
    </>
  );
}
