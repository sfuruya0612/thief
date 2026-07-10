import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { CostExplorerPanel } from './CostExplorerPanel';
import * as endpoints from '../api/endpoints';
import type { CostRaw } from '../types/aws';

// echarts-for-react は jsdom (canvas 未実装) では描画に失敗するため、
// このテストではグラフ描画自体は対象外としダミーコンポーネントに置き換える。
vi.mock('../components/charts/CostChart', () => ({
  CostChart: () => <div data-testid="cost-chart-stub" />,
}));

function raw(
  timePeriod: string,
  service: string,
  unblended: number,
  netAmortized: number,
): CostRaw {
  return {
    time_period: timePeriod,
    service,
    unblended_amount: unblended,
    net_amortized_amount: netAmortized,
    unit: 'USD',
  };
}

const SAMPLE: CostRaw[] = [
  raw('2026-07-01', 'AmazonEC2', 10, 12),
  raw('2026-07-01', 'AmazonS3', 1, 2),
  raw('2026-07-02', 'AmazonEC2', 20, 22),
  raw('2026-07-02', 'AmazonS3', 2, 3),
];

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <CostExplorerPanel profile="test-profile" region="ap-northeast-1" />
    </QueryClientProvider>,
  );
}

describe('CostExplorerPanel', () => {
  it('サービス名フィルタは API を再呼び出しせずブラウザ側でクロス表を絞り込む', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(screen.getByText('AmazonEC2')).toBeInTheDocument());
    expect(screen.getByText('AmazonS3')).toBeInTheDocument();
    const callsBeforeFilter = getCostSpy.mock.calls.length;

    const filterInput = screen.getByPlaceholderText('filter by service name (client-side)…');
    fireEvent.change(filterInput, { target: { value: 'EC2' } });

    await waitFor(() => expect(screen.queryByText('AmazonS3')).not.toBeInTheDocument());
    expect(screen.getByText('AmazonEC2')).toBeInTheDocument();
    // フィルタ入力では getCost が再度呼ばれない (ブラウザ側フィルタのみ)
    expect(getCostSpy.mock.calls.length).toBe(callsBeforeFilter);
  });

  it('開始日/終了日を変更すると getCost が新しい startDate/endDate で呼ばれる', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    const startInput = screen.getAllByTitle('Start date')[0] as HTMLInputElement;
    fireEvent.change(startInput, { target: { value: '2026-06-01' } });

    await waitFor(() => {
      const lastCall = getCostSpy.mock.calls.at(-1);
      expect(lastCall?.[2]?.startDate).toBe('2026-06-01');
    });
  });

  it('クロス表は cost-cross-table クラスで横スクロール可能なテーブルとして描画される', async () => {
    vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(screen.getByText('AmazonEC2')).toBeInTheDocument());
    const table = document.querySelector('table.cost-cross-table');
    expect(table).not.toBeNull();
    const headers = within(table as HTMLElement)
      .getAllByRole('columnheader')
      .map((el) => el.textContent);
    expect(headers).toEqual(['Group', 'Total', '2026-07-01', '2026-07-02']);
  });
});
