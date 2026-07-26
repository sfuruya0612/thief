import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerRDSInstanceParameters } from './DrawerRDSInstanceParameters';

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

function parameterRaw(name: string, value: string) {
  return {
    name,
    value,
    allowed_values: '',
    apply_type: '',
    data_type: '',
    source: '',
    is_modifiable: true,
    description: '',
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

// rds 一覧と rds/parameters をまとめてモックする。parameters を Response 生成関数にして
// エラーケースやグループ別の応答も同じ形で差し込めるようにする。
function mockFetch(list: unknown[], parameters: (url: string) => Promise<Response>) {
  globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString();
    if (url.includes('/rds/parameters')) return parameters(url);
    return okJson(list);
  }) as typeof fetch;
}

describe('DrawerRDSInstanceParameters', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('DB パラメータグループのパラメータを見出し (グループ名) 付きで表示する', async () => {
    mockFetch(
      [rdsListItem({ name: 'db-1', parameter_groups: ['default.mysql8.0'], cluster_id: '' })],
      () => okJson([parameterRaw('max_connections', '100')]),
    );

    const { container } = renderWithQC(
      <DrawerRDSInstanceParameters profile="test" region="ap-northeast-1" instance="db-1" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('max_connections');
    });
    expect(container.textContent).toContain('default.mysql8.0 (1)');
  });

  it('グループが複数のときはセグメントで切り替えられる', async () => {
    mockFetch(
      [rdsListItem({ name: 'db-1', parameter_groups: ['group-a', 'group-b'], cluster_id: '' })],
      (url) =>
        url.includes('group=group-b')
          ? okJson([parameterRaw('param_b', 'B')])
          : okJson([parameterRaw('param_a', 'A')]),
    );

    const { container } = renderWithQC(
      <DrawerRDSInstanceParameters profile="test" region="ap-northeast-1" instance="db-1" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('param_a');
    });

    const segButton = Array.from(container.querySelectorAll('.seg button')).find(
      (b) => b.textContent === 'group-b',
    );
    expect(segButton).not.toBeUndefined();
    fireEvent.click(segButton!);

    await waitFor(() => {
      expect(container.textContent).toContain('param_b');
    });
  });

  it('グループが 0 件のときは No parameter groups. を表示しパラメータ取得を発火させない', async () => {
    mockFetch([rdsListItem({ name: 'db-1', parameter_groups: [], cluster_id: '' })], () =>
      okJson([]),
    );

    const { container } = renderWithQC(
      <DrawerRDSInstanceParameters profile="test" region="ap-northeast-1" instance="db-1" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('No parameter groups.');
    });

    const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;
    const calledParameters = fetchMock.mock.calls.some(([input]) => {
      const url = typeof input === 'string' ? input : (input as URL).toString();
      return url.includes('/rds/parameters');
    });
    expect(calledParameters).toBe(false);
  });

  it('一覧キャッシュに該当行が無い間はローディング表示を出し未取得と 0 件を区別する', async () => {
    mockFetch(
      [rdsListItem({ name: 'db-other', parameter_groups: ['group-a'], cluster_id: '' })],
      () => okJson([]),
    );

    const { container } = renderWithQC(
      <DrawerRDSInstanceParameters profile="test" region="ap-northeast-1" instance="db-1" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Loading…');
    });
    expect(container.textContent).not.toContain('No parameter groups.');
  });

  it('取得エラー時はステータスとコードとメッセージを含むエラーを表示する', async () => {
    mockFetch(
      [rdsListItem({ name: 'db-1', parameter_groups: ['default.mysql8.0'], cluster_id: '' })],
      () =>
        Promise.resolve({
          ok: false,
          status: 403,
          statusText: 'Forbidden',
          json: async () => ({ error: 'access denied', code: 'ACCESS_DENIED' }),
        } as Response),
    );

    const { container } = renderWithQC(
      <DrawerRDSInstanceParameters profile="test" region="ap-northeast-1" instance="db-1" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Error 403 (ACCESS_DENIED): access denied');
    });
    // data が無いのでテーブルは出さない (空表示と区別する)。
    expect(container.querySelector('table.dt')).toBeNull();
  });
});
