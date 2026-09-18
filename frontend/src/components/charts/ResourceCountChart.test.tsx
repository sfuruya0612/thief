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
const captured = vi.hoisted(() => ({
  series: [] as { name: string }[],
  xRange: undefined as { start: number; end: number } | undefined,
}));

vi.mock('./TimeseriesChart', () => ({
  TimeseriesChart: (props: {
    series: { name: string }[];
    xRange?: { start: number; end: number };
  }) => {
    captured.series = props.series;
    captured.xRange = props.xRange;
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

// WINDOW は backend が返す時間窓 (エポックミリ秒)。7 日分の幅を持たせてある。
const WINDOW = { start: 1_699_395_200_000, end: 1_700_000_000_000 };

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
    captured.xRange = undefined;
    mocks.useResourceTimeseries.mockReturnValue({ data: undefined, isLoading: false, error: null });
  });

  it('既定は 7 日で取得し、欠測を含む系列をそのままグラフへ渡す', () => {
    mocks.useResourceTimeseries.mockReturnValue({
      data: { range: '7d', periodSeconds: 300, start: WINDOW.start, end: WINDOW.end, series },
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

  it('応答の時間窓を X 軸の範囲としてグラフへ渡す', () => {
    mocks.useResourceTimeseries.mockReturnValue({
      data: { range: '7d', periodSeconds: 300, start: WINDOW.start, end: WINDOW.end, series },
      isLoading: false,
      error: null,
    });

    renderChart();

    // 軸の範囲を点の範囲から決めると、点が少ない系列では期間を切り替えても軸が変わらない。
    expect(captured.xRange).toEqual(WINDOW);
  });

  it('応答が無いときは X 軸の範囲を渡さない', () => {
    renderChart();

    expect(captured.xRange).toBeUndefined();
  });

  it('窓が正の幅を持たない応答では X 軸の範囲を渡さない (start / end を返さない古い backend)', () => {
    // 旧形状の応答は正規化で start / end が 0 になる。そのまま渡すと軸が 0 に潰れて
    // 全系列が消えるため、渡さずに点の範囲から軸を決めさせる。
    mocks.useResourceTimeseries.mockReturnValue({
      data: { range: '7d', periodSeconds: 300, start: 0, end: 0, series },
      isLoading: false,
      error: null,
    });

    renderChart();

    expect(captured.series).toEqual(series);
    expect(captured.xRange).toBeUndefined();
  });

  it('caption を渡すとグラフの右下に注記を出す', () => {
    mocks.useResourceTimeseries.mockReturnValue({
      data: { range: '7d', periodSeconds: 300, start: WINDOW.start, end: WINDOW.end, series },
      isLoading: false,
      error: null,
    });

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <ResourceCountChart
          service="ec2"
          profile="prod"
          region="ap-northeast-1"
          title="In-service instances"
          caption="Only instances in an Auto Scaling group"
        />
      </QueryClientProvider>,
    );

    expect(screen.getByText('Only instances in an Auto Scaling group')).toBeInTheDocument();
  });

  it('caption を渡さなければ注記を出さない', () => {
    mocks.useResourceTimeseries.mockReturnValue({
      data: { range: '7d', periodSeconds: 300, start: WINDOW.start, end: WINDOW.end, series },
      isLoading: false,
      error: null,
    });

    renderChart();

    expect(screen.queryByText('Only instances in an Auto Scaling group')).not.toBeInTheDocument();
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
