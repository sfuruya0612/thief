import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Profile } from '../../types/common';
import { AwsActiveSessionCard } from './AwsActiveSessionCard';

afterEach(cleanup);

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
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
  });
});
