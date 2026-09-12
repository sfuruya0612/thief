// DatadogView のエラーバナー出し分け (issue 0166) と cost / dashboards の
// セクション切替 (issue 0169) の検証。
// 資格情報不備 (401 DATADOG_NO_CREDENTIALS) のときだけ DatadogAuthBanner を出し、
// それ以外の Datadog のエラーは従来どおり ErrorBanner に落ちることを確認する。
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DatadogView } from './DatadogView';
import { DATADOG_NO_CREDENTIALS_CODE } from '../../lib/datadogAuthError';
import { ApiError } from '../../types/common';

// コスト取得のフックだけを差し替える。再ログイン導線側 (login/start, login/status) は
// 実装のまま動かし、fetch のモックで応答を与える。
const mocks = vi.hoisted(() => ({
  useDatadogHistorical: vi.fn(),
  useDatadogEstimated: vi.fn(),
}));

// echarts-for-react は jsdom (canvas 未実装) では描画に失敗するため、
// CostExplorerPanel のテストと同じくチャートをスタブに差し替える。
vi.mock('../../components/charts/CostChart', () => ({
  CostChart: () => <div data-testid="cost-chart-stub" />,
}));

// セクション切替の検証が目的なので、Dashboards 側の中身はスタブで足りる
// (中身は DatadogDashboardView.test.tsx で検証する)。
vi.mock('./DatadogDashboardView', () => ({
  DatadogDashboardView: ({ orgId }: { orgId: string }) => (
    <div data-testid="dashboard-view-stub">{orgId}</div>
  ),
}));

vi.mock('../../api/queries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api/queries')>();
  return {
    ...actual,
    useDatadogHistorical: mocks.useDatadogHistorical,
    useDatadogEstimated: mocks.useDatadogEstimated,
  };
});

const startBody = {
  state: 'state-1',
  authorization_url: 'https://app.datadoghq.com/oauth2/v1/authorize?state=state-1',
};

function renderView(orgId = 'suborg1') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <DatadogView orgId={orgId} />
    </QueryClientProvider>,
  );
}

describe('DatadogView のエラーバナー出し分け', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.useDatadogEstimated.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('401 DATADOG_NO_CREDENTIALS では DatadogAuthBanner を表示する', () => {
    const err = new ApiError(401, DATADOG_NO_CREDENTIALS_CODE, 'no usable Datadog credentials');
    mocks.useDatadogHistorical.mockReturnValue({ data: undefined, isLoading: false, error: err });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).toBeInTheDocument();
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
    expect(
      screen.getByText('Datadog の認証情報が使えません。再ログインしてください。'),
    ).toBeInTheDocument();
  });

  it('DATADOG_NO_CREDENTIALS 以外の Datadog エラーでは従来どおり ErrorBanner を表示する', () => {
    const err = new ApiError(500, 'INTERNAL_ERROR', 'datadog historical cost: 403 Forbidden');
    mocks.useDatadogHistorical.mockReturnValue({ data: undefined, isLoading: false, error: err });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).not.toBeInTheDocument();
    const banner = container.querySelector('.error-banner');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent('INTERNAL_ERROR');
    expect(banner).toHaveTextContent('datadog historical cost: 403 Forbidden');
  });

  it('エラーが無ければどちらのバナーも表示しない', () => {
    mocks.useDatadogHistorical.mockReturnValue({ data: [], isLoading: false, error: null });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).not.toBeInTheDocument();
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
  });

  it('表示中の組織をコスト取得の両フックへ渡す', () => {
    // orgId を渡し損ねると、どのタブを開いても親組織のコストが表示される。
    mocks.useDatadogHistorical.mockReturnValue({ data: [], isLoading: false, error: null });

    renderView('suborg2');

    expect(mocks.useDatadogHistorical).toHaveBeenCalledWith(
      'suborg2',
      expect.any(String),
      expect.any(String),
      undefined,
      expect.objectContaining({ enabled: true }),
    );
    expect(mocks.useDatadogEstimated).toHaveBeenCalledWith(
      'suborg2',
      expect.any(String),
      expect.any(String),
      undefined,
      expect.objectContaining({ enabled: true }),
    );
  });

  it('DatadogAuthBanner のログインボタンのクリックでログインフローが始まる', async () => {
    const err = new ApiError(401, DATADOG_NO_CREDENTIALS_CODE, 'no usable Datadog credentials');
    mocks.useDatadogHistorical.mockReturnValue({ data: undefined, isLoading: false, error: err });
    globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      const body = url.includes('/api/datadog/auth/login/start')
        ? startBody
        : { status: 'pending' };
      return { ok: true, status: 200, statusText: '', json: async () => body } as Response;
    }) as unknown as typeof fetch;
    const authWindow = { closed: false, close: vi.fn(), location: { replace: vi.fn() } };
    vi.spyOn(window, 'open').mockReturnValue(authWindow as unknown as Window);

    renderView('suborg2');
    fireEvent.click(screen.getByRole('button', { name: 'Datadog 再ログイン' }));

    await waitFor(() =>
      expect(authWindow.location.replace).toHaveBeenCalledWith(startBody.authorization_url),
    );
    expect(screen.getByRole('button', { name: 'ログイン中…' })).toBeDisabled();
    // 再ログインの対象は表示中の組織。
    const startUrl = vi
      .mocked(globalThis.fetch)
      .mock.calls.map((call) => String(call[0]))
      .find((url) => url.includes('/api/datadog/auth/login/start'));
    expect(startUrl).toContain('org=suborg2');
  });
});

describe('DatadogView のセクション切替', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.useDatadogHistorical.mockReturnValue({ data: [], isLoading: false, error: null });
    mocks.useDatadogEstimated.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  it('既定では cost を表示する', () => {
    renderView();
    expect(screen.getByTestId('cost-chart-stub')).toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-view-stub')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Historical' })).toBeInTheDocument();
  });

  it('Dashboards を押すと表示中の組織で Dashboards に切り替わる', () => {
    renderView('suborg2');
    fireEvent.click(screen.getByRole('button', { name: 'Dashboards' }));

    expect(screen.getByTestId('dashboard-view-stub')).toHaveTextContent('suborg2');
    expect(screen.queryByTestId('cost-chart-stub')).not.toBeInTheDocument();
    // cost 専用の Historical / Estimated 切替は dashboards では出さない。
    expect(screen.queryByRole('button', { name: 'Historical' })).not.toBeInTheDocument();
  });

  it('Dashboards に切り替えると Cost の historical/estimated 取得を止める', () => {
    renderView();
    fireEvent.click(screen.getByRole('button', { name: 'Dashboards' }));

    const lastHistoricalCall = mocks.useDatadogHistorical.mock.calls.at(-1);
    const lastEstimatedCall = mocks.useDatadogEstimated.mock.calls.at(-1);
    expect(lastHistoricalCall?.at(-1)).toEqual(expect.objectContaining({ enabled: false }));
    expect(lastEstimatedCall?.at(-1)).toEqual(expect.objectContaining({ enabled: false }));
  });

  it('Cost を押すと cost に戻る', () => {
    renderView();
    fireEvent.click(screen.getByRole('button', { name: 'Dashboards' }));
    fireEvent.click(screen.getByRole('button', { name: 'Cost' }));

    expect(screen.getByTestId('cost-chart-stub')).toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-view-stub')).not.toBeInTheDocument();
  });
});
