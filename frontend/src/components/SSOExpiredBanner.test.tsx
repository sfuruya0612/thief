import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { SSOExpiredBanner } from './SSOExpiredBanner';

afterEach(cleanup);

function renderWithQC() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <SSOExpiredBanner profile="sso-prof" />
    </QueryClientProvider>,
  );
}

describe('SSOExpiredBanner', () => {
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('ログイン中はブラウザでの認可完了を促すヒントを表示する', async () => {
    globalThis.fetch = vi.fn(() => new Promise(() => {})) as unknown as typeof fetch;
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    await waitFor(() =>
      expect(screen.getByText(/ブラウザで認可を完了してください/)).toBeInTheDocument(),
    );
    expect(screen.getByRole('button', { name: 'ログイン中…' })).toBeDisabled();
  });

  it('backend がブラウザ認可完了まで応答しない設計のため、成功時は完了後にのみヒントが消える', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
      statusText: 'No Content',
      json: async () => undefined,
    } as Response) as unknown as typeof fetch;
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    await waitFor(() =>
      expect(screen.getByRole('button', { name: 'SSO 再ログイン' })).toBeInTheDocument(),
    );
    expect(screen.queryByText(/ブラウザで認可を完了してください/)).not.toBeInTheDocument();
  });

  it('ログイン失敗時はエラーメッセージを表示する', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: async () => ({ error: 'sso login failed', code: 'SSO_LOGIN_FAILED' }),
    } as Response) as unknown as typeof fetch;
    renderWithQC();

    fireEvent.click(screen.getByRole('button', { name: 'SSO 再ログイン' }));

    await waitFor(() => expect(screen.getByText('再ログインに失敗しました。')).toBeInTheDocument());
  });
});
