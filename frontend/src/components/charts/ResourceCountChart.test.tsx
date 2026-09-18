// ResourceCountChart の期間切替・読み込み中表示・エラー表示・系列の受け渡しの検証。
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ResourceCountChart } from './ResourceCountChart';
import { ApiError } from '../../types/common';
import type { TimeseriesSeries } from '../../lib/timeseries';

const mocks = vi.hoisted(() => ({ useResourceTimeseries: vi.fn() }));

// echarts-for-react は jsdom (canvas 未実装) では描画できないため、渡された系列を
// 捕まえるスタブに差し替える。
const captured = vi.hoisted(() => ({ series: [] as { name: string }[] }));

vi.mock('./TimeseriesChart', () => ({
  TimeseriesChart: (props: { series: { name: string }[] }) => {
    captured.series = props.series;
    return <div data-testid="timeseries-chart-stub" />;
  },
}));

vi.mock('../../api/queries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api/queries')>();
  return { ...actual, useResourceTimeseries: mocks.useResourceTimeseries };
});

const series: TimeseriesSeries[] = [
  {
    name: 'prod-cluster',
    points: [
      { t: 1_700_000_000_000, v: 3 },
      { t: 1_700_000_060_000, v: null },
    ],
  },
];

function renderChart() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ResourceCountChart
        service="ecs"
        profile="prod"
        region="ap-northeast-1"
        title="Tasks per cluster"
      />
    </QueryClientProvider>,
  );
}

describe('ResourceCountChart', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    captured.series = [];
    mocks.useResourceTimeseries.mockReturnValue({ data: undefined, isLoading: false, error: null });
  });

  it('既定は 7 日で取得し、欠測を含む系列をそのままグラフへ渡す', () => {
    mocks.useResourceTimeseries.mockReturnValue({
      data: { range: '7d', periodSeconds: 300, series },
      isLoading: false,
      error: null,
    });

    renderChart();

    expect(mocks.useResourceTimeseries).toHaveBeenCalledWith('ecs', 'prod', 'ap-northeast-1', '7d');
    expect(screen.getByRole('button', { name: '7 days' })).toHaveClass('active');
    expect(screen.getByRole('button', { name: '1 day' })).not.toHaveClass('active');
    expect(screen.getByText('Tasks per cluster')).toBeInTheDocument();
    expect(screen.getByTestId('timeseries-chart-stub')).toBeInTheDocument();
    // 欠測は null のまま渡す (0 に潰すと「台数が 0 だった」と読めてしまう)。
    expect(captured.series).toEqual(series);
  });

  it('期間ボタンで 1 日と 1 か月に切り替えて取得し直す', () => {
    renderChart();

    fireEvent.click(screen.getByRole('button', { name: '1 day' }));
    expect(mocks.useResourceTimeseries).toHaveBeenLastCalledWith(
      'ecs',
      'prod',
      'ap-northeast-1',
      '1d',
    );

    fireEvent.click(screen.getByRole('button', { name: '1 month' }));
    expect(mocks.useResourceTimeseries).toHaveBeenLastCalledWith(
      'ecs',
      'prod',
      'ap-northeast-1',
      '30d',
    );
  });

  it('取得中はグラフの代わりに読み込み中を出す', () => {
    mocks.useResourceTimeseries.mockReturnValue({ data: undefined, isLoading: true, error: null });

    renderChart();

    expect(screen.getByText('Loading timeseries…')).toBeInTheDocument();
    expect(screen.queryByTestId('timeseries-chart-stub')).not.toBeInTheDocument();
  });

  it('権限不足 (403 ACCESS_DENIED) は理由を出しグラフ領域を出さない', () => {
    mocks.useResourceTimeseries.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new ApiError(
        403,
        'ACCESS_DENIED',
        'User: arn:aws:sts::123456789012:assumed-role/x is not authorized to perform: cloudwatch:GetMetricData',
      ),
    });

    renderChart();

    expect(screen.getByText('403 ACCESS_DENIED')).toBeInTheDocument();
    expect(screen.getByText(/cloudwatch:GetMetricData/)).toBeInTheDocument();
    expect(screen.queryByTestId('timeseries-chart-stub')).not.toBeInTheDocument();
  });
});
