// DatadogDashboardView の一覧表示・選択・ウィジェット描画 (実データ)・未対応フォールバックの検証。
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DatadogDashboardView } from './DatadogDashboardView';
import { DATADOG_NO_CREDENTIALS_CODE } from '../../lib/datadogAuthError';
import { ApiError } from '../../types/common';
import type {
  DatadogDashboardDetailRow,
  DatadogMetricSeriesRow,
  DatadogWidgetRow,
} from '../../types/nonaws';

const mocks = vi.hoisted(() => ({
  useDatadogDashboards: vi.fn(),
  useDatadogDashboard: vi.fn(),
  useDatadogMetricsQueries: vi.fn(),
}));

// echarts-for-react は jsdom (canvas 未実装) では描画できないため、渡された系列を
// 捕まえるスタブに差し替える。
const captured = vi.hoisted(() => ({ series: [] as { name: string }[] }));

vi.mock('../../components/charts/TimeseriesChart', () => ({
  TimeseriesChart: (props: { series: { name: string }[] }) => {
    captured.series = props.series;
    return <div data-testid="timeseries-chart-stub" />;
  },
}));

vi.mock('../../api/queries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api/queries')>();
  return {
    ...actual,
    useDatadogDashboards: mocks.useDatadogDashboards,
    useDatadogDashboard: mocks.useDatadogDashboard,
    useDatadogMetricsQueries: mocks.useDatadogMetricsQueries,
  };
});

const dashboards = [
  {
    id: 'abc-123',
    title: 'Overview',
    description: 'main',
    url: 'https://app.datadoghq.com/dashboard/abc-123/overview',
  },
  { id: 'def-456', title: 'Latency', description: '', url: '' },
];

const widgets: DatadogWidgetRow[] = [
  {
    id: 1,
    kind: 'timeseries',
    type: 'timeseries',
    title: 'CPU',
    queries: ['avg:system.cpu.user{*}'],
  },
  { id: 2, kind: 'query_value', type: 'query_value', title: 'Load', queries: ['sum:x{*}'] },
  { id: 3, kind: 'unsupported', type: 'toplist', title: 'Top', queries: [] },
];

const detail: DatadogDashboardDetailRow = {
  id: 'abc-123',
  title: 'Overview',
  description: 'main',
  url: 'https://app.datadoghq.com/dashboard/abc-123/overview',
  widgets,
};

const cpuSeries: DatadogMetricSeriesRow[] = [
  {
    name: 'avg:system.cpu.user{host:web-1}',
    scope: 'host:web-1',
    unit: '%',
    points: [
      { t: 1_700_000_000_000, v: 12.5 },
      { t: 1_700_000_060_000, v: 13.5 },
    ],
  },
];

function renderView(orgId = 'suborg1') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <DatadogDashboardView orgId={orgId} />
    </QueryClientProvider>,
  );
}

describe('DatadogDashboardView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    captured.series = [];
    mocks.useDatadogDashboards.mockReturnValue({
      data: dashboards,
      isLoading: false,
      error: null,
    });
    mocks.useDatadogDashboard.mockReturnValue({ data: undefined, isLoading: false, error: null });
    mocks.useDatadogMetricsQueries.mockReturnValue({ series: [], isLoading: false, error: null });
  });

  it('ダッシュボード一覧を選択肢として表示する', () => {
    renderView();
    expect(screen.getByRole('option', { name: 'Overview' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Latency' })).toBeInTheDocument();
    expect(screen.getByText('Select a dashboard to see its widgets')).toBeInTheDocument();
  });

  it('ダッシュボードを選ぶと選択した id で詳細を取りに行く', () => {
    renderView();
    fireEvent.change(screen.getByLabelText('Dashboard'), { target: { value: 'def-456' } });
    expect(mocks.useDatadogDashboard).toHaveBeenLastCalledWith('suborg1', 'def-456');
  });

  it('timeseries ウィジェットはクエリの実行結果をグラフへ渡す', () => {
    mocks.useDatadogDashboard.mockReturnValue({ data: detail, isLoading: false, error: null });
    mocks.useDatadogMetricsQueries.mockReturnValue({
      series: cpuSeries,
      isLoading: false,
      error: null,
    });
    renderView();

    expect(screen.getByTestId('timeseries-chart-stub')).toBeInTheDocument();
    expect(screen.getByText('CPU')).toBeInTheDocument();
    // 定義 (クエリ文字列) ではなく実データが渡ること。
    expect(captured.series).toEqual([
      {
        name: 'avg:system.cpu.user{host:web-1}',
        points: [
          { t: 1_700_000_000_000, v: 12.5 },
          { t: 1_700_000_060_000, v: 13.5 },
        ],
      },
    ]);
  });

  it('ウィジェットのクエリと組織と時間窓をメトリクス取得へ渡す', () => {
    mocks.useDatadogDashboard.mockReturnValue({ data: detail, isLoading: false, error: null });
    renderView('suborg2');

    expect(mocks.useDatadogMetricsQueries).toHaveBeenCalledWith(
      'suborg2',
      ['avg:system.cpu.user{*}'],
      expect.objectContaining({ from: expect.any(Number), to: expect.any(Number) }),
    );
    expect(mocks.useDatadogMetricsQueries).toHaveBeenCalledWith(
      'suborg2',
      ['sum:x{*}'],
      expect.anything(),
    );
  });

  it('query_value ウィジェットは最後に観測された値と単位を出す', () => {
    mocks.useDatadogDashboard.mockReturnValue({ data: detail, isLoading: false, error: null });
    mocks.useDatadogMetricsQueries.mockReturnValue({
      series: [
        {
          name: 'sum:x{*}',
          scope: '',
          unit: 'req/s',
          points: [
            { t: 1, v: 10 },
            { t: 2, v: 42 },
            // 末端の欠測は飛ばして、直前の確定値を出す。
            { t: 3, v: null },
          ],
        },
      ],
      isLoading: false,
      error: null,
    });
    renderView();

    expect(screen.getByText('Load')).toBeInTheDocument();
    expect(screen.getByText('42')).toBeInTheDocument();
    expect(screen.getByText('req/s')).toBeInTheDocument();
  });

  it('メトリクス取得中の query_value は値を出さずダッシュのまま待つ', () => {
    mocks.useDatadogDashboard.mockReturnValue({ data: detail, isLoading: false, error: null });
    mocks.useDatadogMetricsQueries.mockReturnValue({
      series: [{ name: 'sum:x{*}', scope: '', unit: '', points: [{ t: 1, v: 42 }] }],
      isLoading: true,
      error: null,
    });
    renderView();

    expect(screen.queryByText('42')).not.toBeInTheDocument();
    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('メトリクス取得に失敗したウィジェットは理由を出す', () => {
    mocks.useDatadogDashboard.mockReturnValue({ data: detail, isLoading: false, error: null });
    mocks.useDatadogMetricsQueries.mockReturnValue({
      series: [],
      isLoading: false,
      error: new ApiError(500, 'INTERNAL_ERROR', 'query datadog metrics: Invalid query'),
    });
    renderView();

    // timeseries と query_value の 2 つが同じ理由で失敗する。
    expect(screen.getAllByText(/Invalid query/)).toHaveLength(2);
    expect(screen.queryByTestId('timeseries-chart-stub')).not.toBeInTheDocument();
  });

  it('未対応ウィジェットは種別名と Datadog へのリンクを出す', () => {
    mocks.useDatadogDashboard.mockReturnValue({ data: detail, isLoading: false, error: null });
    renderView();

    expect(screen.getByText('thief does not render "toplist" widgets.')).toBeInTheDocument();
    const link = screen.getByRole('link', { name: 'Open in Datadog' });
    expect(link).toHaveAttribute('href', 'https://app.datadoghq.com/dashboard/abc-123/overview');
  });

  it('ウィジェットが 1 つも無いダッシュボードでもその旨を出す', () => {
    mocks.useDatadogDashboard.mockReturnValue({
      data: { ...detail, widgets: [] },
      isLoading: false,
      error: null,
    });
    renderView();
    expect(screen.getByText('This dashboard has no widgets')).toBeInTheDocument();
  });

  it('ダッシュボードが 1 つも無い組織ではその旨を出す', () => {
    mocks.useDatadogDashboards.mockReturnValue({ data: [], isLoading: false, error: null });
    renderView();
    expect(screen.getByText('No dashboards in this organization')).toBeInTheDocument();
  });

  it('401 DATADOG_NO_CREDENTIALS では再ログイン導線を出す', () => {
    mocks.useDatadogDashboards.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new ApiError(401, DATADOG_NO_CREDENTIALS_CODE, 'no usable Datadog credentials'),
    });
    renderView();
    expect(screen.getByRole('button', { name: /Datadog/ })).toBeInTheDocument();
  });

  it('それ以外のエラーは ErrorBanner に落ちる', () => {
    mocks.useDatadogDashboards.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new ApiError(500, 'INTERNAL_ERROR', 'list datadog dashboards: boom'),
    });
    renderView();
    expect(screen.getByText(/list datadog dashboards: boom/)).toBeInTheDocument();
  });
});
