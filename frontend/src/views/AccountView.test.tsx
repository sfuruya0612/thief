// ServicePanel (AccountView) のエラーバナー出し分けの検証 (issue 0073)。
// SSO トークン期限切れのときだけ SSOExpiredBanner を出し、それ以外の ApiError
// (403 ACCESS_DENIED 等) は ErrorBanner に落ちることを確認する。
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AccountView } from './AccountView';
import { ApiError } from '../types/common';
import { SSO_TOKEN_EXPIRED_CODE } from '../lib/ssoError';

// ServicePanel が使う useResources / useCost だけを差し替え、他のフック
// (Sidebar の useRegions など) は実装のまま使う。fetch は解決しない Promise に
// して副作用のリクエストを止める。
const mocks = vi.hoisted(() => ({
  useResources: vi.fn(),
  useCost: vi.fn(),
}));

vi.mock('../api/queries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/queries')>();
  return {
    ...actual,
    useResources: mocks.useResources,
    useCost: mocks.useCost,
  };
});

// viewElement は検証対象の AccountView を QueryClientProvider で包んだ要素を返す。
// 初回の render と、props を変えずに再描画する rerender の両方で同じ要素を使う。
function viewElement(client: QueryClient) {
  return (
    <QueryClientProvider client={client}>
      <AccountView
        profile="test-profile"
        region="ap-northeast-1"
        profiles={[]}
        onRegionChange={() => {}}
        activeService="ec2"
        onServiceChange={() => {}}
        drawerPos="right"
      />
    </QueryClientProvider>
  );
}

function renderView(qc?: QueryClient) {
  const client = qc ?? new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(viewElement(client));
}

describe('AccountView のエラーバナー出し分け', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
    mocks.useCost.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  it('403 ACCESS_DENIED の ApiError では SSOExpiredBanner を出さず ErrorBanner に内容を表示する', () => {
    const err = new ApiError(
      403,
      'ACCESS_DENIED',
      'User is not authorized to perform: ec2:DescribeInstances',
    );
    mocks.useResources.mockReturnValue({ data: undefined, isLoading: false, error: err });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).not.toBeInTheDocument();
    const banner = container.querySelector('.error-banner');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent('403');
    expect(banner).toHaveTextContent('ACCESS_DENIED');
    expect(banner).toHaveTextContent('User is not authorized to perform: ec2:DescribeInstances');
  });

  it('401 SSO トークン期限切れの ApiError では SSOExpiredBanner を表示する', () => {
    const err = new ApiError(401, SSO_TOKEN_EXPIRED_CODE, 'SSO token expired');
    mocks.useResources.mockReturnValue({ data: undefined, isLoading: false, error: err });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).toBeInTheDocument();
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
  });

  it('エラーが無ければどちらのバナーも表示しない', () => {
    mocks.useResources.mockReturnValue({ data: [], isLoading: false, error: null });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).not.toBeInTheDocument();
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
  });
});

describe('AccountView の一覧取得に追随した時系列の再取得', () => {
  const TIMESERIES_KEY = ['aws', 'ec2', 'test-profile', 'ap-northeast-1', 'timeseries'];

  function newClient() {
    return new QueryClient({ defaultOptions: { queries: { retry: false } } });
  }

  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
    mocks.useCost.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  it('一覧の取得が成功したら同じ profile と region の時系列クエリを無効化する', () => {
    mocks.useResources.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      dataUpdatedAt: 1_700_000_000_000,
    });
    const qc = newClient();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');

    renderView(qc);

    expect(invalidate).toHaveBeenCalledWith({ queryKey: TIMESERIES_KEY });
  });

  it('一覧が未取得 (dataUpdatedAt が 0) の間は時系列を無効化しない', () => {
    mocks.useResources.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      dataUpdatedAt: 0,
    });
    const qc = newClient();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');

    renderView(qc);

    expect(invalidate).not.toHaveBeenCalledWith({ queryKey: TIMESERIES_KEY });
  });

  it('一覧が取り直されて dataUpdatedAt が進むたびに時系列を無効化する', () => {
    mocks.useResources.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      dataUpdatedAt: 1_700_000_000_000,
    });
    const qc = newClient();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');

    const { rerender } = renderView(qc);
    const before = invalidate.mock.calls.length;

    // Refresh で一覧を取り直した状況。時系列の取得は一覧より先に終わるため、
    // 一覧の更新に追随して取り直さないとその回の記録がグラフに入らない。
    mocks.useResources.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      dataUpdatedAt: 1_700_000_060_000,
    });
    rerender(viewElement(qc));

    expect(invalidate.mock.calls.length).toBeGreaterThan(before);
    expect(invalidate).toHaveBeenLastCalledWith({ queryKey: TIMESERIES_KEY });
  });

  it('一覧の取り直しが失敗して dataUpdatedAt が進まなければ時系列を無効化し直さない', () => {
    mocks.useResources.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      dataUpdatedAt: 1_700_000_000_000,
    });
    const qc = newClient();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');

    const { rerender } = renderView(qc);
    // ServicePanel は呼び出しごとに新しい配列を組むため、参照ではなく中身で数える。
    const timeseriesCalls = () =>
      invalidate.mock.calls.filter(
        ([arg]) => JSON.stringify(arg?.queryKey) === JSON.stringify(TIMESERIES_KEY),
      ).length;
    expect(timeseriesCalls()).toBe(1);

    // 取得済みの一覧を持ったまま再取得に失敗した状況。TanStack Query は失敗した
    // 再取得で dataUpdatedAt を進めず、前回の data と error が並ぶ。backend は
    // 一覧の取得に失敗すると台数を記録しないため、時系列を取り直す必要も無い。
    mocks.useResources.mockReturnValue({
      data: [],
      isLoading: false,
      error: new ApiError(500, 'INTERNAL', 'DescribeInstances failed'),
      dataUpdatedAt: 1_700_000_000_000,
    });
    rerender(viewElement(qc));

    expect(timeseriesCalls()).toBe(1);
  });
});
