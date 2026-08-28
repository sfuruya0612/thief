import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Profile } from '../../types/common';
import { AwsActiveSessionCard } from './AwsActiveSessionCard';

afterEach(cleanup);

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return { qc, ...render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>) };
}

function jsonResponse(status: number, body?: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: '',
    json: async () => body,
  } as Response;
}

// ログアウト関連のエンドポイント別に応答を差し替える fetch モック。それ以外
// (useProfileIdentity の STS 補完) は解決させない。呼び出し順の検証用に、fetch された
// メソッドと URL (パス + クエリ) を calls に積む。
function mockLogoutFetch(handlers: {
  logout?: () => Response | Promise<Response>;
  invalidate?: () => Response | Promise<Response>;
}) {
  const calls: string[] = [];
  globalThis.fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input));
    const path = `${init?.method ?? 'GET'} ${url.pathname}${url.search}`;
    if (path === 'POST /api/aws/profiles/p/sso/logout') {
      calls.push(path);
      return handlers.logout ? handlers.logout() : jsonResponse(204);
    }
    if (path === 'POST /api/cache/invalidate?view=aws') {
      calls.push(path);
      return handlers.invalidate ? handlers.invalidate() : jsonResponse(204);
    }
    return new Promise<Response>(() => {});
  }) as unknown as typeof fetch;
  return calls;
}

const inFuture = (minutes: number) => new Date(Date.now() + minutes * 60_000).toISOString();

describe('AwsActiveSessionCard', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    // useProfileIdentity (STS 補完) は解決させないままにする
    globalThis.fetch = vi.fn(() => new Promise(() => {})) as unknown as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('プロファイル名・認証方式・Account ID・バッジを表示する', () => {
    const profiles: Profile[] = [
      {
        name: 'sso-prof',
        accountId: '111111111111',
        ssoRoleName: 'AdministratorAccess',
        authType: 'sso',
        ssoStatus: 'valid',
        ssoExpiresAt: inFuture(120),
      },
    ];
    renderWithQC(<AwsActiveSessionCard profile="sso-prof" profiles={profiles} />);
    expect(screen.getByText('sso-prof')).toBeInTheDocument();
    expect(screen.getByText('SSO · AdministratorAccess')).toBeInTheDocument();
    expect(screen.getByText('111111111111')).toBeInTheDocument();
    expect(screen.getByText('SSO 有効')).toBeInTheDocument();
  });

  it('STS で確定した Account ID が来たら上書き表示する', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => ({ account_id: '999999999999', arn: 'arn:x', user_id: 'AID' }),
    } as Response) as unknown as typeof fetch;
    renderWithQC(
      <AwsActiveSessionCard
        profile="sso-prof"
        profiles={[{ name: 'sso-prof', accountId: '111111111111' }]}
      />,
    );
    await waitFor(() => expect(screen.getByText('999999999999')).toBeInTheDocument());
  });

  it('期限が十分先なら通常色の残り時間を表示し再認証行は出ない', () => {
    const profiles: Profile[] = [
      { name: 'p', authType: 'sso', ssoStatus: 'valid', ssoExpiresAt: inFuture(125) },
    ];
    renderWithQC(<AwsActiveSessionCard profile="p" profiles={profiles} />);
    const expiry = screen.getByText(/残り 2 時間/);
    expect(expiry).not.toHaveClass('expiring');
    expect(screen.queryByText('aws sso login --profile p')).not.toBeInTheDocument();
  });

  it('期限間近は橙表示になり再認証コマンドが出る', () => {
    const profiles: Profile[] = [
      { name: 'p', authType: 'sso', ssoStatus: 'valid', ssoExpiresAt: inFuture(10) },
    ];
    renderWithQC(<AwsActiveSessionCard profile="p" profiles={profiles} />);
    expect(screen.getByText(/残り \d+ 分/)).toHaveClass('expiring');
    expect(screen.getByText('aws sso login --profile p')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'コピー' })).toBeInTheDocument();
  });

  it('期限切れバッジでも再認証コマンドが出る', () => {
    const profiles: Profile[] = [{ name: 'p', authType: 'sso', ssoStatus: 'expired' }];
    renderWithQC(<AwsActiveSessionCard profile="p" profiles={profiles} />);
    expect(screen.getByText('期限切れ')).toBeInTheDocument();
    expect(screen.getByText('aws sso login --profile p')).toBeInTheDocument();
  });

  it('一覧に無いプロファイルでも名前だけで描画できる', () => {
    renderWithQC(<AwsActiveSessionCard profile="ghost" profiles={[]} />);
    expect(screen.getByText('ghost')).toBeInTheDocument();
    expect(screen.getByText('-')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'SSO ログアウト' })).not.toBeInTheDocument();
  });

  describe('SSO ログアウト', () => {
    const validSso = (): Profile => ({
      name: 'p',
      authType: 'sso',
      ssoStatus: 'valid',
      ssoExpiresAt: inFuture(125),
    });
    const logoutButton = () => screen.getByRole('button', { name: 'SSO ログアウト' });

    it('ssoStatus が valid の SSO profile にはログアウトボタンが出る (title に他 profile への影響を記載)', () => {
      renderWithQC(<AwsActiveSessionCard profile="p" profiles={[validSso()]} />);
      const button = logoutButton();
      expect(button).toBeEnabled();
      expect(button).toHaveAttribute(
        'title',
        'この profile の SSO セッションを AWS 側で失効し、トークンキャッシュを削除します。同じ start URL を共有する他の profile も未ログインになります',
      );
    });

    it.each<[string, Profile]>([
      ['expired', { name: 'p', authType: 'sso', ssoStatus: 'expired' }],
      ['not_logged_in', { name: 'p', authType: 'sso', ssoStatus: 'not_logged_in' }],
      ['ssoStatus 欠落', { name: 'p', authType: 'sso', ssoExpiresAt: inFuture(125) }],
      ['非 SSO', { name: 'p', authType: 'access_key', ssoStatus: 'valid' }],
    ])('%s のときはログアウトボタンを出さない', (_label, profile) => {
      renderWithQC(<AwsActiveSessionCard profile="p" profiles={[profile]} />);
      expect(screen.queryByRole('button', { name: 'SSO ログアウト' })).not.toBeInTheDocument();
    });

    it('valid かつ期限間近ならログアウトボタンと再認証コマンドのコピー導線が両方出る', () => {
      renderWithQC(
        <AwsActiveSessionCard
          profile="p"
          profiles={[{ ...validSso(), ssoExpiresAt: inFuture(10) }]}
        />,
      );
      expect(logoutButton()).toBeInTheDocument();
      expect(screen.getByText('aws sso login --profile p')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'コピー' })).toBeInTheDocument();
    });

    it('押下で POST .../sso/logout が呼ばれ、成功後は backend キャッシュ破棄が 1 回先行してから [aws] を 1 回無効化する', async () => {
      const calls = mockLogoutFetch({});
      const { qc } = renderWithQC(<AwsActiveSessionCard profile="p" profiles={[validSso()]} />);
      const invalidate = vi.spyOn(qc, 'invalidateQueries');
      // fetch と invalidateQueries を 1 本の時系列に並べて順序を検証する
      invalidate.mockImplementation(async (filters) => {
        calls.push(`invalidateQueries:${JSON.stringify(filters?.queryKey)}`);
      });

      fireEvent.click(logoutButton());

      // mutation の完了 (isPending が解けてボタンが戻る) まで待ち、後続の invalidate 追加も取りこぼさない
      await waitFor(() => expect(logoutButton()).toBeEnabled());
      expect(invalidate).toHaveBeenCalledTimes(1);
      expect(calls).toEqual([
        'POST /api/aws/profiles/p/sso/logout',
        'POST /api/cache/invalidate?view=aws',
        'invalidateQueries:["aws"]',
      ]);
    });

    it('ログアウト中はボタンが disabled になる', async () => {
      mockLogoutFetch({ logout: () => new Promise(() => {}) });
      renderWithQC(<AwsActiveSessionCard profile="p" profiles={[validSso()]} />);

      fireEvent.click(logoutButton());

      await waitFor(() =>
        expect(screen.getByRole('button', { name: 'ログアウト中…' })).toBeDisabled(),
      );
    });

    it('別の profile に切り替えると前の profile の失敗表示を持ち越さない', async () => {
      mockLogoutFetch({
        logout: () => jsonResponse(500, { error: 'boom', code: 'SSO_LOGOUT_FAILED' }),
      });
      const profiles = [validSso(), { ...validSso(), name: 'q' }];
      const { rerender, qc } = renderWithQC(
        <AwsActiveSessionCard profile="p" profiles={profiles} />,
      );

      fireEvent.click(logoutButton());
      await waitFor(() => expect(screen.getByText('ログアウトに失敗しました')).toBeInTheDocument());

      rerender(
        <QueryClientProvider client={qc}>
          <AwsActiveSessionCard profile="q" profiles={profiles} />
        </QueryClientProvider>,
      );
      await waitFor(() =>
        expect(screen.queryByText('ログアウトに失敗しました')).not.toBeInTheDocument(),
      );
      expect(logoutButton()).toBeEnabled();
    });

    it('同じ profile のまま再レンダーしても失敗表示は消えない', async () => {
      // reset は profile の切り替え時だけ走り、同一 profile の再レンダーでは走らないことを固定する
      mockLogoutFetch({
        logout: () => jsonResponse(500, { error: 'boom', code: 'SSO_LOGOUT_FAILED' }),
      });
      const profiles = [validSso()];
      const { rerender, qc } = renderWithQC(
        <AwsActiveSessionCard profile="p" profiles={profiles} />,
      );

      fireEvent.click(logoutButton());
      await waitFor(() => expect(screen.getByText('ログアウトに失敗しました')).toBeInTheDocument());

      rerender(
        <QueryClientProvider client={qc}>
          <AwsActiveSessionCard profile="p" profiles={[...profiles]} />
        </QueryClientProvider>,
      );
      expect(screen.getByText('ログアウトに失敗しました')).toBeInTheDocument();
    });

    it('ログアウト成功後も backend キャッシュ破棄が終わるまではボタンが disabled のまま', async () => {
      // onSuccess (refresher) の Promise が解決するまで isPending が続くことを固定する
      const calls = mockLogoutFetch({ invalidate: () => new Promise(() => {}) });
      renderWithQC(<AwsActiveSessionCard profile="p" profiles={[validSso()]} />);

      fireEvent.click(logoutButton());

      await waitFor(() => expect(calls).toContain('POST /api/cache/invalidate?view=aws'));
      expect(screen.getByRole('button', { name: 'ログアウト中…' })).toBeDisabled();
    });

    it('失敗 (500) 時は backend キャッシュを破棄せず、固定の失敗文言だけを出し、ボタンは再度押せる', async () => {
      const calls = mockLogoutFetch({
        logout: () =>
          // backend の ErrorResponse は {error, code, details}。error にメッセージが入る
          jsonResponse(500, {
            error: 'remove sso cache file x.json: permission denied',
            code: 'SSO_LOGOUT_FAILED',
          }),
      });
      const { qc } = renderWithQC(<AwsActiveSessionCard profile="p" profiles={[validSso()]} />);
      const invalidate = vi.spyOn(qc, 'invalidateQueries');

      fireEvent.click(logoutButton());

      await waitFor(() => expect(screen.getByText('ログアウトに失敗しました')).toBeInTheDocument());
      expect(screen.queryByText(/permission denied/)).not.toBeInTheDocument();
      expect(calls).toEqual(['POST /api/aws/profiles/p/sso/logout']);
      expect(invalidate).not.toHaveBeenCalled();
      expect(logoutButton()).toBeEnabled();

      fireEvent.click(logoutButton());
      await waitFor(() =>
        expect(calls).toEqual([
          'POST /api/aws/profiles/p/sso/logout',
          'POST /api/aws/profiles/p/sso/logout',
        ]),
      );
      await waitFor(() => expect(screen.getByText('ログアウトに失敗しました')).toBeInTheDocument());
      expect(logoutButton()).toBeEnabled();
    });
  });
});
