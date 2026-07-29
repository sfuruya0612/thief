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
  it('フィルタへの入力だけでは getCost を再呼び出ししない', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(screen.getByText('AmazonEC2')).toBeInTheDocument());
    const callsBeforeInput = getCostSpy.mock.calls.length;

    fireEvent.change(screen.getByPlaceholderText('filter by service name…'), {
      target: { value: 'AmazonEC2' },
    });
    fireEvent.change(screen.getByPlaceholderText('filter by account ID…'), {
      target: { value: '123456789012' },
    });

    // 入力値は state に反映されるが、確定前なので API は呼ばれない
    await waitFor(() =>
      expect(screen.getByPlaceholderText('filter by service name…')).toHaveValue('AmazonEC2'),
    );
    expect(getCostSpy.mock.calls.length).toBe(callsBeforeInput);
  });

  it('Enter の押下でサービス名フィルタが確定し getCost が service 付きで呼ばれる', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    const serviceInput = screen.getByPlaceholderText('filter by service name…');
    // 前後の空白は確定時に除去される
    fireEvent.change(serviceInput, { target: { value: '  AmazonEC2  ' } });
    fireEvent.keyDown(serviceInput, { key: 'Enter' });

    await waitFor(() => {
      const lastCall = getCostSpy.mock.calls.at(-1);
      expect(lastCall?.[2]?.service).toBe('AmazonEC2');
    });
    // サービス名の確定がアカウント ID 側の確定値を巻き込まないこと
    expect(getCostSpy.mock.calls.at(-1)?.[2]?.account).toBe('');
    // 入力欄の表示値も trim 後の値に揃うこと。表示だけ空白付きで残ると、実際に絞り込みへ
    // 使われる値と画面の表示が食い違って見える。
    expect(serviceInput).toHaveValue('AmazonEC2');
  });

  it('フォーカス離脱でアカウント ID フィルタが確定し getCost が account 付きで呼ばれる', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    const accountInput = screen.getByPlaceholderText('filter by account ID…');
    fireEvent.change(accountInput, { target: { value: '123456789012' } });
    fireEvent.blur(accountInput);

    await waitFor(() => {
      const lastCall = getCostSpy.mock.calls.at(-1);
      expect(lastCall?.[2]?.account).toBe('123456789012');
    });
    // アカウント ID の確定がサービス名側の確定値を巻き込まないこと
    expect(getCostSpy.mock.calls.at(-1)?.[2]?.service).toBe('');
  });

  it('Enter で確定した直後にフォーカス離脱しても getCost は再呼び出しされない', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    const serviceInput = screen.getByPlaceholderText('filter by service name…');
    fireEvent.change(serviceInput, { target: { value: 'AmazonEC2' } });
    fireEvent.keyDown(serviceInput, { key: 'Enter' });

    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.service).toBe('AmazonEC2'));
    const callsAfterEnter = getCostSpy.mock.calls.length;

    // Enter と blur の両方が確定を発火させるため、Enter 直後の blur では同じ値で確定が 2 回走る。
    // 確定値が変わらなければ queryKey も変わらないので、有償 API の再呼び出しは起きてはならない。
    fireEvent.blur(serviceInput);

    await waitFor(() => expect(serviceInput).toHaveValue('AmazonEC2'));
    expect(getCostSpy.mock.calls.length).toBe(callsAfterEnter);
  });

  it('確定済みのフィルタを空文字にして確定すると絞り込みが解除される', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    const serviceInput = screen.getByPlaceholderText('filter by service name…');
    fireEvent.change(serviceInput, { target: { value: 'AmazonEC2' } });
    fireEvent.keyDown(serviceInput, { key: 'Enter' });
    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.service).toBe('AmazonEC2'));

    // 空文字での確定は絞り込み解除として扱う (パラメータを省略するのではなく空文字を送る)
    fireEvent.change(serviceInput, { target: { value: '' } });
    fireEvent.keyDown(serviceInput, { key: 'Enter' });

    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.service).toBe(''));
  });

  it('Group by の変更は確定済みのフィルタを巻き込まない', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    const serviceInput = screen.getByPlaceholderText('filter by service name…');
    fireEvent.change(serviceInput, { target: { value: 'AmazonEC2' } });
    fireEvent.keyDown(serviceInput, { key: 'Enter' });
    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.service).toBe('AmazonEC2'));

    const accountInput = screen.getByPlaceholderText('filter by account ID…');
    fireEvent.change(accountInput, { target: { value: '123456789012' } });
    fireEvent.blur(accountInput);
    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.account).toBe('123456789012'));

    // 集計軸の切り替えは絞り込み条件とは独立した state であり、GetCostAndUsage には
    // GroupBy と Filter の両方が同時に渡り続けなければならない
    fireEvent.change(screen.getByTitle('Group by'), { target: { value: 'USAGE_TYPE' } });

    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.groupBy).toBe('USAGE_TYPE'));
    const lastCall = getCostSpy.mock.calls.at(-1);
    expect(lastCall?.[2]?.service).toBe('AmazonEC2');
    expect(lastCall?.[2]?.account).toBe('123456789012');
    // 入力欄の表示値も維持されること
    expect(serviceInput).toHaveValue('AmazonEC2');
    expect(accountInput).toHaveValue('123456789012');
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
