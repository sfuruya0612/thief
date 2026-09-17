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
  useDatadogMetricsQueries: vi.fn(),
}));

// echarts-for-react は jsdom (canvas 未実装) では描画に失敗するため、
// CostExplorerPanel のテストと同じくチャートをスタブに差し替える。
vi.mock('../../components/charts/CostChart', () => ({
  CostChart: () => <div data-testid="cost-chart-stub" />,
}));

vi.mock('../../components/charts/TimeseriesChart', () => ({
  TimeseriesChart: () => <div data-testid="timeseries-chart-stub" />,
}));

// Dashboards の中身は DatadogDashboardView.test.tsx で検証するため、ここはスタブで足りる。
// スタブは引き継ぎ導線だけを模し、押されたら固定のクエリを親へ返す。
// Metrics は入力状態の保持 (issue 0180) を検証する対象なので実体を描画する。
vi.mock('./DatadogDashboardView', () => ({
  DatadogDashboardView: ({
    orgId,
    onOpenQuery,
  }: {
    orgId: string;
    onOpenQuery: (query: string) => void;
  }) => (
    <div data-testid="dashboard-view-stub">
      {orgId}
      <button onClick={() => onOpenQuery('avg:system.cpu.user{*}')}>Open in Metrics</button>
    </div>
  ),
}));

vi.mock('../../api/queries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../api/queries')>();
  return {
    ...actual,
    useDatadogHistorical: mocks.useDatadogHistorical,
    useDatadogEstimated: mocks.useDatadogEstimated,
    useDatadogMetricsQueries: mocks.useDatadogMetricsQueries,
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
    mocks.useDatadogMetricsQueries.mockReturnValue({ series: [], isLoading: false, error: null });
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

  it('Metrics を押すと表示中の組織で Metrics に切り替わる', () => {
    renderView('suborg2');
    fireEvent.click(screen.getByRole('button', { name: 'Metrics' }));

    expect(screen.getByLabelText('Query')).toBeInTheDocument();
    expect(screen.queryByTestId('cost-chart-stub')).not.toBeInTheDocument();
    expect(screen.queryByTestId('dashboard-view-stub')).not.toBeInTheDocument();
    // 直接開いた Metrics には引き継ぐクエリが無い。
    expect(screen.getByLabelText('Query')).toHaveValue('');
    expect(mocks.useDatadogMetricsQueries).toHaveBeenLastCalledWith(
      'suborg2',
      [],
      expect.anything(),
    );
  });

  it('Metrics に切り替えると Cost の historical/estimated 取得を止める', () => {
    renderView();
    fireEvent.click(screen.getByRole('button', { name: 'Metrics' }));

    expect(mocks.useDatadogHistorical.mock.calls.at(-1)?.at(-1)).toEqual(
      expect.objectContaining({ enabled: false }),
    );
    expect(mocks.useDatadogEstimated.mock.calls.at(-1)?.at(-1)).toEqual(
      expect.objectContaining({ enabled: false }),
    );
  });

  it('Dashboards の引き継ぎ導線で Metrics へクエリを渡して切り替わる', () => {
    renderView();
    fireEvent.click(screen.getByRole('button', { name: 'Dashboards' }));
    fireEvent.click(screen.getByRole('button', { name: 'Open in Metrics' }));

    expect(screen.queryByTestId('dashboard-view-stub')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Query')).toHaveValue('avg:system.cpu.user{*}');
    expect(mocks.useDatadogMetricsQueries).toHaveBeenLastCalledWith(
      'suborg1',
      ['avg:system.cpu.user{*}'],
      expect.anything(),
    );
  });
});

describe('DatadogView をまたぐ Metrics の入力の保持', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.useDatadogHistorical.mockReturnValue({ data: [], isLoading: false, error: null });
    mocks.useDatadogEstimated.mockReturnValue({ data: [], isLoading: false, error: null });
    mocks.useDatadogMetricsQueries.mockReturnValue({ series: [], isLoading: false, error: null });
  });

  it('入力中のクエリは Cost へ移って戻っても残る', () => {
    renderView();
    fireEvent.click(screen.getByRole('button', { name: 'Metrics' }));
    fireEvent.change(screen.getByLabelText('Query'), {
      target: { value: 'avg:system.cpu.idle{*}' },
    });

    fireEvent.click(screen.getByRole('button', { name: 'Cost' }));
    fireEvent.click(screen.getByRole('button', { name: 'Metrics' }));

    expect(screen.getByLabelText('Query')).toHaveValue('avg:system.cpu.idle{*}');
  });

  it('選んだ期間は Dashboards へ移って戻っても残る', () => {
    renderView();
    fireEvent.click(screen.getByRole('button', { name: 'Metrics' }));
    fireEvent.change(screen.getByLabelText('Period'), { target: { value: String(24 * 3600) } });

    fireEvent.click(screen.getByRole('button', { name: 'Dashboards' }));
    fireEvent.click(screen.getByRole('button', { name: 'Metrics' }));

    expect(screen.getByLabelText('Period')).toHaveValue(String(24 * 3600));
  });

  it('実行済みのクエリは Cost へ移って戻っても実行済みのまま', () => {
    renderView();
    fireEvent.click(screen.getByRole('button', { name: 'Metrics' }));
    fireEvent.change(screen.getByLabelText('Query'), { target: { value: 'sum:x{*}' } });
    fireEvent.click(screen.getByRole('button', { name: 'Run' }));

    fireEvent.click(screen.getByRole('button', { name: 'Cost' }));
    fireEvent.click(screen.getByRole('button', { name: 'Metrics' }));

    expect(mocks.useDatadogMetricsQueries).toHaveBeenLastCalledWith(
      'suborg1',
      ['sum:x{*}'],
      expect.anything(),
    );
  });

  it('Run を押していないクエリは戻っても実行されない', () => {
    renderView();
    fireEvent.click(screen.getByRole('button', { name: 'Metrics' }));
    fireEvent.change(screen.getByLabelText('Query'), { target: { value: 'sum:x{*}' } });

    fireEvent.click(screen.getByRole('button', { name: 'Cost' }));
    fireEvent.click(screen.getByRole('button', { name: 'Metrics' }));

    expect(screen.getByLabelText('Query')).toHaveValue('sum:x{*}');
    expect(mocks.useDatadogMetricsQueries).toHaveBeenLastCalledWith(
      'suborg1',
      [],
      expect.anything(),
    );
  });
});
