import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerCacheParameters } from './DrawerCacheParameters';

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

function okJson(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    statusText: 'OK',
    json: async () => body,
  } as Response;
}

function cacheRaw(parameterGroup: string) {
  return {
    id: 'my-cluster-001',
    name: 'my-cluster',
    state: 'available',
    engine: 'redis',
    engine_version: '7.1.0',
    node_type: 'cache.t4g.micro',
    num_nodes: 1,
    endpoint: 'my-cluster.abc.apne1.cache.amazonaws.com',
    port: 6379,
    parameter_group: parameterGroup,
    replication_group_id: '',
    cost_monthly: 0,
  };
}

const parameterRaw = {
  name: 'maxmemory-policy',
  value: 'volatile-lru',
  allowed_values: 'volatile-lru,allkeys-lru',
  change_type: 'immediate',
  data_type: 'string',
  source: 'user',
  is_modifiable: true,
  minimum_engine_version: '',
};

describe('DrawerCacheParameters', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('一覧キャッシュ未取得の間は No parameter group. ではなくローディングを表示する', async () => {
    // 一覧 query を未解決のままにして「未取得」状態を維持する
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockReturnValue(new Promise(() => {}));

    const { container } = renderWithQC(
      <DrawerCacheParameters profile="test" region="ap-northeast-1" cluster="my-cluster" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Loading…');
    });
    expect(container.textContent).not.toContain('No parameter group.');
  });

  it('パラメータグループを持たないクラスタでは No parameter group. を表示する', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockResolvedValue(okJson([cacheRaw('')]));

    const { container } = renderWithQC(
      <DrawerCacheParameters profile="test" region="ap-northeast-1" cluster="my-cluster" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('No parameter group.');
    });
  });

  it('パラメータグループが解決できたらパラメータ一覧を表示する', async () => {
    (globalThis.fetch as ReturnType<typeof vi.fn>).mockImplementation(
      async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url.includes('/elasticache/parameters')) return okJson([parameterRaw]);
        if (url.includes('/elasticache')) return okJson([cacheRaw('default.redis7')]);
        throw new Error(`unexpected fetch: ${url}`);
      },
    );

    const { container } = renderWithQC(
      <DrawerCacheParameters profile="test" region="ap-northeast-1" cluster="my-cluster" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('default.redis7 (1)');
    });
    expect(container.textContent).toContain('maxmemory-policy');
  });
});
