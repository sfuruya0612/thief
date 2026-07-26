import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerRDSClusterParameters } from './DrawerRDSClusterParameters';

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

function okJson(body: unknown): Promise<Response> {
  return Promise.resolve({
    ok: true,
    status: 200,
    statusText: 'OK',
    json: async () => body,
  } as Response);
}

// rds 一覧と rds/cluster-parameters をまとめてモックする。
function mockFetch(list: unknown[], clusterParameters: () => Promise<Response>) {
  globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString();
    if (url.includes('/rds/cluster-parameters')) return clusterParameters();
    return okJson(list);
  }) as typeof fetch;
}

describe('DrawerRDSClusterParameters', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('クラスターパラメータを Cluster: プレフィックスなしの clusterId 見出しで表示する', async () => {
    mockFetch(
      [
        rdsListItem({
          name: 'db-1',
          parameter_groups: ['default.aurora-mysql8.0'],
          cluster_id: 'aurora-cluster-1',
        }),
      ],
      () =>
        okJson([
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
        ]),
    );

    const { container } = renderWithQC(
      <DrawerRDSClusterParameters profile="test" region="ap-northeast-1" instance="db-1" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('binlog_format');
    });
    expect(container.textContent).toContain('aurora-cluster-1 (1)');
    expect(container.textContent).not.toContain('Cluster:');
  });

  it('クラスターに属さないインスタンスは Not part of a DB cluster. を表示し取得を発火させない', async () => {
    mockFetch(
      [rdsListItem({ name: 'db-1', parameter_groups: ['default.mysql8.0'], cluster_id: '' })],
      () => okJson([]),
    );

    const { container } = renderWithQC(
      <DrawerRDSClusterParameters profile="test" region="ap-northeast-1" instance="db-1" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Not part of a DB cluster.');
    });

    const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;
    const calledClusterParameters = fetchMock.mock.calls.some(([input]) => {
      const url = typeof input === 'string' ? input : (input as URL).toString();
      return url.includes('/rds/cluster-parameters');
    });
    expect(calledClusterParameters).toBe(false);
  });

  it('一覧キャッシュに該当行が無い間はローディング表示を出し空表示と区別する', async () => {
    mockFetch(
      [rdsListItem({ name: 'db-other', parameter_groups: [], cluster_id: 'aurora-cluster-1' })],
      () => okJson([]),
    );

    const { container } = renderWithQC(
      <DrawerRDSClusterParameters profile="test" region="ap-northeast-1" instance="db-1" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Loading…');
    });
    expect(container.textContent).not.toContain('Not part of a DB cluster.');
  });

  it('取得エラー時はステータスとコードとメッセージを含むエラーを表示する', async () => {
    mockFetch(
      [
        rdsListItem({
          name: 'db-1',
          parameter_groups: ['default.aurora-mysql8.0'],
          cluster_id: 'aurora-cluster-1',
        }),
      ],
      () =>
        Promise.resolve({
          ok: false,
          status: 403,
          statusText: 'Forbidden',
          json: async () => ({ error: 'access denied', code: 'ACCESS_DENIED' }),
        } as Response),
    );

    const { container } = renderWithQC(
      <DrawerRDSClusterParameters profile="test" region="ap-northeast-1" instance="db-1" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Error 403 (ACCESS_DENIED): access denied');
    });
    // data が無いのでテーブルは出さない (空表示と区別する)。
    expect(container.querySelector('table.dt')).toBeNull();
  });
});
