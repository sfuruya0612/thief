import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DatadogAuthBanner } from './DatadogAuthBanner';
import { DATADOG_LOGIN_POLL_TIMEOUT } from '../hooks/useDatadogLogin';

afterEach(cleanup);

function renderWithQC(org = 'suborg1') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <DatadogAuthBanner org={org} />
    </QueryClientProvider>,
  );
}

// start 応答のボディ (backend の datadogLoginStartResponse 相当)。
const startBody = {
  state: 'state-1',
  authorization_url: 'https://app.datadoghq.com/oauth2/v1/authorize?state=state-1',
};

function jsonResponse(status: number, body?: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: '',
    json: async () => body,
  } as Response;
}

// login/start と login/status のエンドポイント別に応答を差し替える fetch モック。
function mockFetch(handlers: {
  start?: () => Response | Promise<Response>;
  status?: () => Response | Promise<Response>;
}) {
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.includes('/api/datadog/auth/login/start')) {
      return handlers.start ? handlers.start() : jsonResponse(200, startBody);
    }
    if (url.includes('/api/datadog/auth/login/status')) {
      return handlers.status ? handlers.status() : jsonResponse(200, { status: 'pending' });
    }
    throw new Error(`unexpected fetch: ${url}`);
  }) as unknown as typeof fetch;
}

// fetch モックが受け取った URL のうち login/start のものを返す。
function startRequestUrls(): string[] {
  return vi
    .mocked(globalThis.fetch)
    .mock.calls.map((call) => String(call[0]))
    .filter((url) => url.includes('/api/datadog/auth/login/start'));
}

// 認可タブの WindowProxy のモック。closed は「ユーザが手動で閉じた後か」を表す。
function mockAuthWindow(closed = false) {
  const authWindow = { closed, close: vi.fn(), location: { replace: vi.fn() } };
  vi.spyOn(window, 'open').mockReturnValue(authWindow as unknown as Window);
  return authWindow;
}

// 偽タイマーを使うテストでの時間の経過と再取得の解決 (useDatadogLogin.test.tsx と同じ形)。
async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

describe('DatadogAuthBanner', () => {
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('資格情報不備の案内とログインボタンを表示する', () => {
    renderWithQC();

    expect(
      screen.getByText('Datadog の認証情報が使えません。再ログインしてください。'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Datadog 再ログイン' })).toBeEnabled();
  });

  it('ログインボタンのクリックで認可タブを認可 URL へ遷移させ、認可完了を促すヒントを表示する', async () => {
    mockFetch({});
    const authWindow = mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'Datadog 再ログイン' }));

    await waitFor(() =>
      expect(authWindow.location.replace).toHaveBeenCalledWith(startBody.authorization_url),
    );
    expect(screen.getByText(/ブラウザで認可を完了してください/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'ログイン中…' })).toBeDisabled();
    expect(authWindow.close).not.toHaveBeenCalled();
  });

  it('ログインの対象を表示中の組織に限定する (org を login/start へ渡す)', async () => {
    // org を渡し損ねると、Sub Organization のタブから始めたログインが親組織の
    // トークンを上書きし、そのタブは未ログインのままになる。
    mockFetch({});
    mockAuthWindow();
    renderWithQC('suborg1');

    fireEvent.click(screen.getByRole('button', { name: 'Datadog 再ログイン' }));

    await waitFor(() => expect(startRequestUrls().length).toBe(1));
    expect(startRequestUrls()[0]).toContain('org=suborg1');
  });

  it('親組織 (org が空文字) では org クエリを付けない', async () => {
    mockFetch({});
    mockAuthWindow();
    renderWithQC('');

    fireEvent.click(screen.getByRole('button', { name: 'Datadog 再ログイン' }));

    await waitFor(() => expect(startRequestUrls().length).toBe(1));
    expect(startRequestUrls()[0]).not.toContain('org=');
  });

  it('login/status が succeeded になったら認可タブを閉じてヒントを消す', async () => {
    mockFetch({ status: () => jsonResponse(200, { status: 'succeeded' }) });
    const authWindow = mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'Datadog 再ログイン' }));

    await waitFor(() => expect(authWindow.close).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'Datadog 再ログイン' })).toBeEnabled(),
    );
    expect(screen.queryByText(/ブラウザで認可を完了してください/)).not.toBeInTheDocument();
  });

  it('login/status が 404 を返したら打ち切りを待たず失敗表示にする', async () => {
    mockFetch({
      status: () =>
        jsonResponse(404, {
          error: 'datadog login session not found or expired; start a new login',
          code: 'DATADOG_LOGIN_SESSION_NOT_FOUND',
        }),
    });
    mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'Datadog 再ログイン' }));

    await waitFor(() =>
      expect(screen.getByText('Datadog の再ログインに失敗しました。')).toBeInTheDocument(),
    );
    expect(screen.getByRole('button', { name: 'Datadog 再ログイン' })).toBeEnabled();
  });

  it('打ち切り時間まで pending が続いたら失敗表示に切り替える', async () => {
    vi.useFakeTimers();
    mockFetch({});
    mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'Datadog 再ログイン' }));

    await advance(1);
    expect(screen.getByText(/ブラウザで認可を完了してください/)).toBeInTheDocument();

    // 打ち切り時刻を越えた後の最初のポーリングでログインフローがエラー終了する。
    await advance(DATADOG_LOGIN_POLL_TIMEOUT);
    await advance(1);
    expect(screen.getByText('Datadog の再ログインに失敗しました。')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Datadog 再ログイン' })).toBeEnabled();
  });

  it('login/start が 409 を返したら再登録が必要である旨の専用メッセージを表示する', async () => {
    mockFetch({
      start: () =>
        jsonResponse(409, {
          error: 'the registered datadog oauth client does not have this redirect uri',
          code: 'DATADOG_REDIRECT_URI_NOT_REGISTERED',
        }),
    });
    const authWindow = mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'Datadog 再ログイン' }));

    await waitFor(() =>
      expect(screen.getByText(/リダイレクト URI が登録されていません/)).toBeInTheDocument(),
    );
    // 通常のログイン失敗の文言は出さない。認可タブへの遷移も行わない。
    expect(screen.queryByText('Datadog の再ログインに失敗しました。')).not.toBeInTheDocument();
    expect(authWindow.location.replace).not.toHaveBeenCalled();
    expect(authWindow.close).toHaveBeenCalledTimes(1);
  });

  it('ポップアップブロック時は認可 URL をリンクとして表示する', async () => {
    mockFetch({});
    vi.spyOn(window, 'open').mockReturnValue(null);
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'Datadog 再ログイン' }));

    const link = await screen.findByRole('link', { name: '認可ページ' });
    expect(link).toHaveAttribute('href', startBody.authorization_url);
  });
});
