import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AccountView } from './AccountView';
import { CostExplorerPanel } from './CostExplorerPanel';
import * as endpoints from '../api/endpoints';
import { SSO_TOKEN_EXPIRED_CODE } from '../lib/ssoError';
import type { CostRaw } from '../types/aws';
import { ApiError } from '../types/common';

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
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('フィルタへの入力だけでは getCost を再呼び出ししない', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(screen.getByText('AmazonEC2')).toBeInTheDocument());
    const callsBeforeInput = getCostSpy.mock.calls.length;

    fireEvent.change(screen.getByPlaceholderText('filter by service / usage type / account…'), {
      target: { value: 'AmazonEC2' },
    });

    // 入力値は state に反映されるが、確定前なので API は呼ばれない
    await waitFor(() =>
      expect(screen.getByPlaceholderText('filter by service / usage type / account…')).toHaveValue(
        'AmazonEC2',
      ),
    );
    expect(getCostSpy.mock.calls.length).toBe(callsBeforeInput);
  });

  it('Enter の押下でキーワードが確定し getCost が keyword 付きで呼ばれる', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    const keywordInput = screen.getByPlaceholderText('filter by service / usage type / account…');
    // 前後の空白は確定時に除去される
    fireEvent.change(keywordInput, { target: { value: '  AmazonEC2  ' } });
    fireEvent.keyDown(keywordInput, { key: 'Enter' });

    await waitFor(() => {
      const lastCall = getCostSpy.mock.calls.at(-1);
      expect(lastCall?.[2]?.keyword).toBe('AmazonEC2');
    });
    // 入力欄の表示値も trim 後の値に揃うこと。表示だけ空白付きで残ると、実際に絞り込みへ
    // 使われる値と画面の表示が食い違って見える。
    expect(keywordInput).toHaveValue('AmazonEC2');
  });

  it('フォーカス離脱でキーワードが確定し getCost が keyword 付きで呼ばれる', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    // アカウント ID のようにサービス名以外の次元へ向けた値も、同じ 1 つの入力欄から渡す。
    const keywordInput = screen.getByPlaceholderText('filter by service / usage type / account…');
    fireEvent.change(keywordInput, { target: { value: '123456789012' } });
    fireEvent.blur(keywordInput);

    await waitFor(() => {
      const lastCall = getCostSpy.mock.calls.at(-1);
      expect(lastCall?.[2]?.keyword).toBe('123456789012');
    });
  });

  it('Enter で確定した直後にフォーカス離脱しても getCost は再呼び出しされない', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    const keywordInput = screen.getByPlaceholderText('filter by service / usage type / account…');
    fireEvent.change(keywordInput, { target: { value: 'AmazonEC2' } });
    fireEvent.keyDown(keywordInput, { key: 'Enter' });

    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.keyword).toBe('AmazonEC2'));
    const callsAfterEnter = getCostSpy.mock.calls.length;

    // Enter と blur の両方が確定を発火させるため、Enter 直後の blur では同じ値で確定が 2 回走る。
    // 確定値が変わらなければ queryKey も変わらないので、有償 API の再呼び出しは起きてはならない。
    fireEvent.blur(keywordInput);

    await waitFor(() => expect(keywordInput).toHaveValue('AmazonEC2'));
    expect(getCostSpy.mock.calls.length).toBe(callsAfterEnter);
  });

  it('確定済みのフィルタを空文字にして確定すると絞り込みが解除される', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    const keywordInput = screen.getByPlaceholderText('filter by service / usage type / account…');
    fireEvent.change(keywordInput, { target: { value: 'AmazonEC2' } });
    fireEvent.keyDown(keywordInput, { key: 'Enter' });
    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.keyword).toBe('AmazonEC2'));

    // 空文字での確定は絞り込み解除として扱う (パラメータを省略するのではなく空文字を送る)
    fireEvent.change(keywordInput, { target: { value: '' } });
    fireEvent.keyDown(keywordInput, { key: 'Enter' });

    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.keyword).toBe(''));
  });

  it('Group by の変更は確定済みのフィルタを巻き込まない', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    renderPanel();

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    const keywordInput = screen.getByPlaceholderText('filter by service / usage type / account…');
    fireEvent.change(keywordInput, { target: { value: 'AmazonEC2' } });
    fireEvent.keyDown(keywordInput, { key: 'Enter' });
    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.keyword).toBe('AmazonEC2'));

    // 集計軸の切り替えは絞り込み条件とは独立した state であり、GetCostAndUsage には
    // GroupBy と Filter の両方が同時に渡り続けなければならない
    fireEvent.change(screen.getByTitle('Group by'), { target: { value: 'USAGE_TYPE' } });

    await waitFor(() => expect(getCostSpy.mock.calls.at(-1)?.[2]?.groupBy).toBe('USAGE_TYPE'));
    expect(getCostSpy.mock.calls.at(-1)?.[2]?.keyword).toBe('AmazonEC2');
    // 入力欄の表示値も維持されること
    expect(keywordInput).toHaveValue('AmazonEC2');
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

  it('AccountView 経由でリージョンを切り替えると絞り込み state が全て初期値に戻る', async () => {
    const getCostSpy = vi.spyOn(endpoints, 'getCost').mockResolvedValue(SAMPLE);
    // Sidebar の useRegions などコスト以外の副作用リクエストは、AccountView.test.tsx と
    // 同じ方式で解決しない Promise に差し替えて止める。
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise(() => {})),
    );

    // 実装箇所である AccountView をそのままレンダリングし、key={region} による再マウントの
    // 経路を通す。key を持たない props の差し替えだけでは useState は初期化されないため、
    // AccountView 側の key={region} が外れるとこのテストは落ちる。
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const view = (region: string) => (
      <QueryClientProvider client={queryClient}>
        <AccountView
          profile="test-profile"
          region={region}
          profiles={[]}
          onRegionChange={() => {}}
          activeService="costexplorer"
          onServiceChange={() => {}}
          drawerPos="right"
        />
      </QueryClientProvider>
    );
    const { rerender } = render(view('ap-northeast-1'));

    await waitFor(() => expect(getCostSpy).toHaveBeenCalled());

    // 初期値を控えておく (日付レンジは現在日時から計算されるため固定値を書かない)
    const initialStart = (screen.getByTitle('Start date') as HTMLInputElement).value;
    const initialEnd = (screen.getByTitle('End date') as HTMLInputElement).value;

    // 7 つの state を全て初期値以外へ変更する
    const keywordInput = screen.getByPlaceholderText('filter by service / usage type / account…');
    fireEvent.change(keywordInput, { target: { value: 'AmazonEC2' } });
    fireEvent.keyDown(keywordInput, { key: 'Enter' });
    fireEvent.change(screen.getByTitle('Start date'), { target: { value: '2026-06-01' } });
    fireEvent.change(screen.getByTitle('End date'), { target: { value: '2026-07-15' } });
    fireEvent.change(screen.getByTitle('Granularity'), { target: { value: 'MONTHLY' } });
    fireEvent.change(screen.getByTitle('Group by'), { target: { value: 'USAGE_TYPE' } });
    fireEvent.change(screen.getByTitle('Cost metric'), { target: { value: 'netAmortized' } });

    // 変更が state に反映されたことを確認してからリージョンを切り替える
    await waitFor(() => {
      const lastCall = getCostSpy.mock.calls.at(-1);
      expect(lastCall?.[2]?.keyword).toBe('AmazonEC2');
      expect(lastCall?.[2]?.granularity).toBe('MONTHLY');
    });
    expect(screen.getByTitle('Cost metric')).toHaveValue('netAmortized');

    rerender(view('us-east-1'));

    // 再マウントにより入力値と確定値の両方が初期値へ戻る
    await waitFor(() => {
      expect(screen.getByPlaceholderText('filter by service / usage type / account…')).toHaveValue(
        '',
      );
    });
    expect(screen.getByTitle('Start date')).toHaveValue(initialStart);
    expect(screen.getByTitle('End date')).toHaveValue(initialEnd);
    expect(screen.getByTitle('Granularity')).toHaveValue('DAILY');
    expect(screen.getByTitle('Group by')).toHaveValue('SERVICE');
    expect(screen.getByTitle('Cost metric')).toHaveValue('unblended');

    // UI に表示されない確定値 (keywordApplied) のリセットは、新しいリージョンでの
    // getCost の呼び出し引数で検証する (それ以外の state はフォーム要素の値に
    // 直接束縛されているため上の検証で足りる)
    await waitFor(() => {
      const lastCall = getCostSpy.mock.calls.at(-1);
      expect(lastCall?.[1]).toBe('us-east-1');
      expect(lastCall?.[2]?.keyword).toBe('');
    });
  });

  it('getCost が 401 SSO_TOKEN_EXPIRED で失敗すると SSOExpiredBanner を表示する', async () => {
    // メッセージは実機で観測した /api/aws/profiles/{profile}/cost の 401 応答と同形にする。
    vi.spyOn(endpoints, 'getCost').mockRejectedValue(
      new ApiError(
        401,
        SSO_TOKEN_EXPIRED_CODE,
        'get cost and usage: operation error Cost Explorer: GetCostAndUsage,' +
          ' get identity: get credentials: failed to refresh cached credentials,' +
          ' the SSO session has expired or is invalid',
      ),
    );
    const { container } = renderPanel();

    await waitFor(() => expect(container.querySelector('.sso-banner')).toBeInTheDocument());
    // SSO 期限切れは汎用の ErrorBanner ではなく再ログイン導線付きのバナーで表示する
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
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
