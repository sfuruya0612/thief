// DatadogMetricsView のクエリ入力・期間指定・グラフ表示・エラー表示の検証。
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { DatadogMetricsView } from './DatadogMetricsView';
import { DATADOG_NO_CREDENTIALS_CODE } from '../../lib/datadogAuthError';
import { ApiError } from '../../types/common';
import type { DatadogMetricSeriesRow } from '../../types/nonaws';

const mocks = vi.hoisted(() => ({ useDatadogMetricsQueries: vi.fn() }));

// echarts-for-react は jsdom (canvas 未実装) では描画できないため、渡された系列と
// 値の書式を捕まえるスタブに差し替える。
const captured = vi.hoisted(() => ({
  series: [] as { name: string }[],
  valueFormatter: undefined as ((v: number) => string) | undefined,
}));

vi.mock('../../components/charts/TimeseriesChart', () => ({
  TimeseriesChart: (props: {
    series: { name: string }[];
    valueFormatter?: (v: number) => string;
  }) => {
    captured.series = props.series;
    captured.valueFormatter = props.valueFormatter;
    return <div data-testid="timeseries-chart-stub" />;
  },
}));

vi.mock('../../api/queries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api/queries')>();
  return { ...actual, useDatadogMetricsQueries: mocks.useDatadogMetricsQueries };
});

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

function renderView(props?: { orgId?: string; initialQuery?: string }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <DatadogMetricsView orgId={props?.orgId ?? 'suborg1'} initialQuery={props?.initialQuery} />
    </QueryClientProvider>,
  );
}

// runQuery はクエリ文字列を入力して Run を押す。
function runQuery(query: string) {
  fireEvent.change(screen.getByLabelText('Query'), { target: { value: query } });
  fireEvent.click(screen.getByRole('button', { name: 'Run' }));
}

describe('DatadogMetricsView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    captured.series = [];
    captured.valueFormatter = undefined;
    mocks.useDatadogMetricsQueries.mockReturnValue({ series: [], isLoading: false, error: null });
  });

  it('クエリ未入力の間は取得もグラフ描画も行わない', () => {
    renderView();

    expect(mocks.useDatadogMetricsQueries).toHaveBeenCalledWith(
      'suborg1',
      [],
      expect.objectContaining({ from: expect.any(Number), to: expect.any(Number) }),
    );
    expect(screen.queryByTestId('timeseries-chart-stub')).not.toBeInTheDocument();
    expect(screen.getByText('Enter a metric query to see its timeseries')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Run' })).toBeDisabled();
  });

  it('入力しただけでは実行せず、Run を押して初めて表示中の組織でクエリを実行する', () => {
    renderView({ orgId: 'suborg2' });

    fireEvent.change(screen.getByLabelText('Query'), { target: { value: 'avg:system.cpu' } });
    expect(mocks.useDatadogMetricsQueries).not.toHaveBeenCalledWith(
      'suborg2',
      ['avg:system.cpu'],
      expect.anything(),
    );

    fireEvent.click(screen.getByRole('button', { name: 'Run' }));
    expect(mocks.useDatadogMetricsQueries).toHaveBeenLastCalledWith(
      'suborg2',
      ['avg:system.cpu'],
      expect.anything(),
    );
  });

  it('Enter キーでもクエリを実行する', () => {
    renderView();

    const input = screen.getByLabelText('Query');
    fireEvent.change(input, { target: { value: 'sum:x{*}' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(mocks.useDatadogMetricsQueries).toHaveBeenLastCalledWith(
      'suborg1',
      ['sum:x{*}'],
      expect.anything(),
    );
  });

  it('前後の空白を落としてから実行する', () => {
    renderView();
    runQuery('  avg:system.cpu.user{*}  ');

    expect(mocks.useDatadogMetricsQueries).toHaveBeenLastCalledWith(
      'suborg1',
      ['avg:system.cpu.user{*}'],
      expect.anything(),
    );
  });

  it('実行した結果の系列と単位付きの書式をグラフへ渡す', () => {
    mocks.useDatadogMetricsQueries.mockReturnValue({
      series: cpuSeries,
      isLoading: false,
      error: null,
    });
    renderView();
    runQuery('avg:system.cpu.user{*}');

    expect(screen.getByTestId('timeseries-chart-stub')).toBeInTheDocument();
    expect(captured.series).toEqual([
      {
        name: 'avg:system.cpu.user{host:web-1}',
        points: [
          { t: 1_700_000_000_000, v: 12.5 },
          { t: 1_700_000_060_000, v: 13.5 },
        ],
      },
    ]);
    expect(captured.valueFormatter?.(12.5)).toBe('12.5 %');
    // 実行中のクエリを見出しに出す。入力を書き換えてもグラフの出自が分かるようにする。
    expect(screen.getByText('avg:system.cpu.user{*}')).toBeInTheDocument();
  });

  it('期間を変えると同じ長さだけ遡った時間窓で取り直す', () => {
    renderView();
    runQuery('avg:system.cpu.user{*}');

    const beforeRange = mocks.useDatadogMetricsQueries.mock.calls.at(-1)?.[2];
    expect(beforeRange.to - beforeRange.from).toBe(3600);

    fireEvent.change(screen.getByLabelText('Period'), { target: { value: String(24 * 3600) } });

    const afterCall = mocks.useDatadogMetricsQueries.mock.calls.at(-1);
    expect(afterCall?.[1]).toEqual(['avg:system.cpu.user{*}']);
    expect(afterCall?.[2].to - afterCall?.[2].from).toBe(24 * 3600);
  });

  it('取得中はグラフを出さずに待つ', () => {
    mocks.useDatadogMetricsQueries.mockReturnValue({
      series: [],
      isLoading: true,
      error: null,
    });
    renderView();
    runQuery('avg:system.cpu.user{*}');

    expect(screen.getByText('Loading…')).toBeInTheDocument();
    expect(screen.queryByTestId('timeseries-chart-stub')).not.toBeInTheDocument();
  });

  it('不正なクエリのエラーは理由を出しグラフを描かない', () => {
    mocks.useDatadogMetricsQueries.mockReturnValue({
      series: [],
      isLoading: false,
      error: new ApiError(400, 'INVALID_ARGUMENT', 'query datadog metrics: Invalid query'),
    });
    renderView();
    runQuery('bogus query');

    expect(screen.getByText(/Invalid query/)).toBeInTheDocument();
    expect(screen.queryByTestId('timeseries-chart-stub')).not.toBeInTheDocument();
  });

  it('401 DATADOG_NO_CREDENTIALS では再ログイン導線を出す', () => {
    mocks.useDatadogMetricsQueries.mockReturnValue({
      series: [],
      isLoading: false,
      error: new ApiError(401, DATADOG_NO_CREDENTIALS_CODE, 'no usable Datadog credentials'),
    });
    const { container } = renderView();
    runQuery('avg:system.cpu.user{*}');

    expect(container.querySelector('.sso-banner')).toBeInTheDocument();
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
  });

  it('引き継いだクエリは入力済み・実行済みの状態で開く', () => {
    mocks.useDatadogMetricsQueries.mockReturnValue({
      series: cpuSeries,
      isLoading: false,
      error: null,
    });
    renderView({ initialQuery: 'avg:system.cpu.user{*}' });

    expect(screen.getByLabelText('Query')).toHaveValue('avg:system.cpu.user{*}');
    expect(mocks.useDatadogMetricsQueries).toHaveBeenCalledWith(
      'suborg1',
      ['avg:system.cpu.user{*}'],
      expect.anything(),
    );
    expect(screen.getByTestId('timeseries-chart-stub')).toBeInTheDocument();
  });
});
