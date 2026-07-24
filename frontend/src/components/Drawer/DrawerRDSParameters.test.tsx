import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerRDSParameters } from './DrawerRDSParameters';

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

function rdsListItem(overrides: { name: string; parameter_groups: string[]; cluster_id: string }) {
  return {
    id: overrides.name,
    name: overrides.name,
    state: 'available',
    engine: 'aurora-mysql',
    engine_version: '8.0.mysql_aurora.3.04.0',
    class: 'db.r6g.large',
    multi_az: false,
    endpoint: 'db.example.com',
    port: 3306,
    vpc_id: 'vpc-1',
    parameter_groups: overrides.parameter_groups,
    cluster_id: overrides.cluster_id,
    tags: {},
    cost_monthly: 0,
    launch_time: '2026-01-01T00:00:00Z',
  };
}

function mockFetch(instance: ReturnType<typeof rdsListItem>) {
  globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString();
    if (url.includes('/rds/cluster-parameters')) {
      return Promise.resolve({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => [
          {
            name: 'binlog_format',
            value: 'ROW',
            allowed_values: '',
            apply_type: '',
            data_type: '',
            source: '',
            is_modifiable: true,
            description: '',
          },
        ],
      } as Response);
    }
    if (url.includes('/rds/parameters')) {
      return Promise.resolve({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => [
          {
            name: 'max_connections',
            value: '100',
            allowed_values: '',
            apply_type: '',
            data_type: '',
            source: '',
            is_modifiable: true,
            description: '',
          },
        ],
      } as Response);
    }
    return Promise.resolve({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => [instance],
    } as Response);
  }) as typeof fetch;
}

describe('DrawerRDSParameters', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('cluster_id を持つインスタンスでは Cluster セグメントが選べ、選択するとクラスターパラメータを取得する', async () => {
    const instance = rdsListItem({
      name: 'db-1',
      parameter_groups: ['default.aurora-mysql8.0'],
      cluster_id: 'aurora-cluster-1',
    });
    mockFetch(instance);

    const { container } = renderWithQC(
      <DrawerRDSParameters profile="test" region="ap-northeast-1" instance="db-1" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('max_connections');
    });

    const clusterButton = Array.from(container.querySelectorAll('button')).find((b) =>
      b.textContent?.startsWith('Cluster:'),
    );
    expect(clusterButton).not.toBeUndefined();

    fireEvent.click(clusterButton!);

    await waitFor(() => {
      expect(container.textContent).toContain('binlog_format');
    });
  });

  it('cluster_id を持たないインスタンスでは Cluster セグメントが表示されずクラスターパラメータ取得を発火させない', async () => {
    const instance = rdsListItem({
      name: 'db-2',
      parameter_groups: ['default.mysql8.0'],
      cluster_id: '',
    });
    mockFetch(instance);

    const { container } = renderWithQC(
      <DrawerRDSParameters profile="test" region="ap-northeast-1" instance="db-2" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('max_connections');
    });

    const clusterButton = Array.from(container.querySelectorAll('button')).find((b) =>
      b.textContent?.startsWith('Cluster:'),
    );
    expect(clusterButton).toBeUndefined();

    const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;
    const calledClusterParameters = fetchMock.mock.calls.some(([input]) => {
      const url = typeof input === 'string' ? input : (input as URL).toString();
      return url.includes('/rds/cluster-parameters');
    });
    expect(calledClusterParameters).toBe(false);
  });

  it('クラスター区分のパラメータ取得が失敗してもインスタンス区分の表示に影響しない', async () => {
    const instance = rdsListItem({
      name: 'db-3',
      parameter_groups: ['default.aurora-mysql8.0'],
      cluster_id: 'aurora-cluster-1',
    });
    globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/rds/cluster-parameters')) {
        return Promise.resolve({
          ok: false,
          status: 403,
          statusText: 'Forbidden',
          json: async () => ({ error: { code: 'access_denied', message: 'access denied' } }),
        } as Response);
      }
      if (url.includes('/rds/parameters')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          statusText: 'OK',
          json: async () => [
            {
              name: 'max_connections',
              value: '100',
              allowed_values: '',
              apply_type: '',
              data_type: '',
              source: '',
              is_modifiable: true,
              description: '',
            },
          ],
        } as Response);
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => [instance],
      } as Response);
    }) as typeof fetch;

    const { container } = renderWithQC(
      <DrawerRDSParameters profile="test" region="ap-northeast-1" instance="db-3" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('max_connections');
    });

    const clusterButton = Array.from(container.querySelectorAll('button')).find((b) =>
      b.textContent?.startsWith('Cluster:'),
    );
    fireEvent.click(clusterButton!);

    // クラスター区分の取得が失敗しても例外にならず、インスタンス区分の値は保持される
    await waitFor(() => {
      expect(container.querySelector('table.dt')).not.toBeNull();
    });
  });
});
