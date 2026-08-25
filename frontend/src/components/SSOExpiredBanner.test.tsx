import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { SSOExpiredBanner } from './SSOExpiredBanner';
import i18n from '../i18n';

afterEach(cleanup);

function renderWithQC() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <SSOExpiredBanner profile="sso-prof" />
    </QueryClientProvider>,
  );
}

// start 応答のボディ (backend の ssoLoginStartResponse 相当)。
const startBody = {
  session_id: 'sess-1',
  verification_uri_complete: 'https://device.sso.example/?user_code=ABCD-EFGH',
  verification_uri: 'https://device.sso.example/',
  user_code: 'ABCD-EFGH',
};

function jsonResponse(status: number, body?: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: '',
    json: async () => body,
  } as Response;
}

// start / complete のエンドポイント別に応答を差し替える fetch モック。
function mockFetch(handlers: {
  start?: () => Response | Promise<Response>;
  complete?: () => Response | Promise<Response>;
}) {
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    if (url.endsWith('/sso/login/start')) {
      return handlers.start ? handlers.start() : jsonResponse(200, startBody);
    }
    if (url.endsWith('/sso/login/complete')) {
      return handlers.complete ? handlers.complete() : jsonResponse(204);
    }
    throw new Error(`unexpected fetch: ${url}`);
  }) as unknown as typeof fetch;
}

// 認可タブの WindowProxy のモックオブジェクト。closed は「ユーザが手動で閉じた
// 後か」を表し、既定は開いたまま (false)。
function newAuthWindow(closed = false) {
  return { closed, close: vi.fn(), location: { replace: vi.fn() } };
}

// window.open が newAuthWindow の戻り値を返すようスパイする。返り値を呼び出しごとに
// 変えたいテストは newAuthWindow と vi.spyOn を直接組み合わせる。
function mockAuthWindow(closed = false) {
  const authWindow = newAuthWindow(closed);
  vi.spyOn(window, 'open').mockReturnValue(authWindow as unknown as Window);
  return authWindow;
}

describe('SSOExpiredBanner', () => {
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('ログイン中は認可タブを認可 URL へ遷移させ、認可完了を促すヒントを表示する', async () => {
    mockFetch({ complete: () => new Promise(() => {}) });
    const authWindow = mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    await waitFor(() =>
      expect(authWindow.location.replace).toHaveBeenCalledWith(startBody.verification_uri_complete),
    );
    expect(screen.getByText(/ブラウザで認可を完了してください/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'ログイン中…' })).toBeDisabled();
    expect(authWindow.close).not.toHaveBeenCalled();
  });

  it('complete 成功時は認可タブを閉じ、ヒントが消える', async () => {
    mockFetch({});
    const authWindow = mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    await waitFor(() => expect(authWindow.close).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'SSO 再ログイン' })).toBeInTheDocument(),
    );
    expect(screen.queryByText(/ブラウザで認可を完了してください/)).not.toBeInTheDocument();
  });

  it('認可拒否 (SSO_LOGIN_ACCESS_DENIED) 時は認可タブを閉じ、失敗を表示する', async () => {
    mockFetch({
      complete: () =>
        jsonResponse(403, { error: 'access denied', code: 'SSO_LOGIN_ACCESS_DENIED' }),
    });
    const authWindow = mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    await waitFor(() => expect(screen.getByText('再ログインに失敗しました。')).toBeInTheDocument());
    // 認可タブへの遷移まで正常に進んだうえでの拒否であることを確認する。
    expect(authWindow.location.replace).toHaveBeenCalledWith(startBody.verification_uri_complete);
    expect(authWindow.close).toHaveBeenCalledTimes(1);
  });

  it('complete がその他の失敗で終わった場合は認可タブを閉じず、失敗を表示する', async () => {
    mockFetch({
      complete: () => jsonResponse(500, { error: 'sso login failed', code: 'SSO_LOGIN_FAILED' }),
    });
    const authWindow = mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    await waitFor(() => expect(screen.getByText('再ログインに失敗しました。')).toBeInTheDocument());
    // 認可タブへの遷移まで正常に進んだうえでの失敗であることを確認する。
    expect(authWindow.location.replace).toHaveBeenCalledWith(startBody.verification_uri_complete);
    expect(authWindow.close).not.toHaveBeenCalled();
  });

  it('start 失敗時は開いた空タブを閉じ、失敗を表示する', async () => {
    mockFetch({
      start: () => jsonResponse(404, { error: 'profile not found', code: 'PROFILE_NOT_FOUND' }),
    });
    const authWindow = mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    await waitFor(() => expect(screen.getByText('再ログインに失敗しました。')).toBeInTheDocument());
    expect(authWindow.close).toHaveBeenCalledTimes(1);
    expect(authWindow.location.replace).not.toHaveBeenCalled();
  });

  it('start 応答時点でユーザが既に空タブを閉じていた場合は location を操作せずフォールバックリンクを表示する', async () => {
    mockFetch({ complete: () => new Promise(() => {}) });
    const authWindow = mockAuthWindow(true);
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    // 閉じたタブの location.replace と close は呼ばれず、ユーザが自分で認可を
    // 継続できるようフォールバックリンクに切り替わることを確認する。
    const link = await screen.findByRole('link', { name: '認可ページ' });
    expect(link).toHaveAttribute('href', startBody.verification_uri_complete);
    expect(authWindow.location.replace).not.toHaveBeenCalled();
    expect(authWindow.close).not.toHaveBeenCalled();
  });

  it('ポップアップブロック時は認可 URL をリンクとして表示する', async () => {
    mockFetch({ complete: () => new Promise(() => {}) });
    vi.spyOn(window, 'open').mockReturnValue(null);
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    const link = await screen.findByRole('link', { name: '認可ページ' });
    expect(link).toHaveAttribute('href', startBody.verification_uri_complete);
  });

  it('verification_uri_complete が無い場合は空タブを閉じ、verification_uri と user_code を表示する', async () => {
    mockFetch({
      start: () => jsonResponse(200, { ...startBody, verification_uri_complete: '' }),
      complete: () => new Promise(() => {}),
    });
    const authWindow = mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    const link = await screen.findByRole('link', { name: '認可ページ' });
    expect(link).toHaveAttribute('href', startBody.verification_uri);
    expect(screen.getByText(/コード ABCD-EFGH を入力してください/)).toBeInTheDocument();
    await waitFor(() => expect(authWindow.close).toHaveBeenCalledTimes(1));
    expect(authWindow.location.replace).not.toHaveBeenCalled();
  });

  it('フォールバック表示は再クリックでリセットされる', async () => {
    // 1 回目はポップアップブロックで、フォールバックリンクの表示を確認してから
    // complete を失敗させる (complete の応答はテスト側から手動で解決する)。
    // 2 回目はタブを開けて complete 待機。
    let resolveFirstComplete!: (r: Response) => void;
    let completeCalls = 0;
    mockFetch({
      complete: () => {
        completeCalls += 1;
        if (completeCalls === 1) {
          return new Promise<Response>((resolve) => {
            resolveFirstComplete = resolve;
          });
        }
        return new Promise(() => {});
      },
    });
    const authWindow = newAuthWindow();
    vi.spyOn(window, 'open')
      .mockReturnValueOnce(null)
      .mockReturnValue(authWindow as unknown as Window);
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    // 1 回目: tabUnavailable が立ち、フォールバックリンクが実際に表示される。
    await screen.findByRole('link', { name: '認可ページ' });
    resolveFirstComplete(
      jsonResponse(500, { error: 'sso login failed', code: 'SSO_LOGIN_FAILED' }),
    );
    await waitFor(() => expect(screen.getByText('再ログインに失敗しました。')).toBeInTheDocument());

    // 2 回目: タブを自動制御できる (tabUnavailable がリセットされる) ため、認可タブへ
    // 遷移し、フォールバックリンクは表示されない。
    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));
    await waitFor(() =>
      expect(authWindow.location.replace).toHaveBeenCalledWith(startBody.verification_uri_complete),
    );
    expect(screen.queryByRole('link', { name: '認可ページ' })).not.toBeInTheDocument();
  });

  it('closed 検知でのフォールバック表示も再クリックでリセットされる', async () => {
    // 1 回目は開いた空タブが start 応答時点で閉じられていた経路 (onTabUnavailable) で
    // フォールバックリンクを表示させ、complete を失敗させる。2 回目は開いたままの
    // タブで complete 待機。
    let resolveFirstComplete!: (r: Response) => void;
    let completeCalls = 0;
    mockFetch({
      complete: () => {
        completeCalls += 1;
        if (completeCalls === 1) {
          return new Promise<Response>((resolve) => {
            resolveFirstComplete = resolve;
          });
        }
        return new Promise(() => {});
      },
    });
    const closedWindow = newAuthWindow(true);
    const openWindow = newAuthWindow();
    vi.spyOn(window, 'open')
      .mockReturnValueOnce(closedWindow as unknown as Window)
      .mockReturnValue(openWindow as unknown as Window);
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    // 1 回目: closed 検知で tabUnavailable が立ち、フォールバックリンクが表示される。
    await screen.findByRole('link', { name: '認可ページ' });
    resolveFirstComplete(
      jsonResponse(500, { error: 'sso login failed', code: 'SSO_LOGIN_FAILED' }),
    );
    await waitFor(() => expect(screen.getByText('再ログインに失敗しました。')).toBeInTheDocument());

    // 2 回目: タブを自動制御できるため、認可タブへ遷移し、フォールバックリンクは
    // 表示されない (closed 経路で立った tabUnavailable もリセットされる)。
    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));
    await waitFor(() =>
      expect(openWindow.location.replace).toHaveBeenCalledWith(startBody.verification_uri_complete),
    );
    expect(screen.queryByRole('link', { name: '認可ページ' })).not.toBeInTheDocument();
  });

  it('英語表示時は英語の文言で表示する (issue 0050)', async () => {
    await i18n.changeLanguage('en');
    renderWithQC();

    expect(screen.getByText(/has expired\. Please log in again\./)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'SSO re-login' })).toBeInTheDocument();
  });

  it('英語表示時はフォールバックのリンクと user_code も英語で表示する', async () => {
    await i18n.changeLanguage('en');
    mockFetch({
      start: () => jsonResponse(200, { ...startBody, verification_uri_complete: '' }),
      complete: () => new Promise(() => {}),
    });
    mockAuthWindow();
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO re-login' }));

    const link = await screen.findByRole('link', { name: 'authorization page' });
    expect(link).toHaveAttribute('href', startBody.verification_uri);
    expect(screen.getByText(/Enter code ABCD-EFGH/)).toBeInTheDocument();
  });
});
