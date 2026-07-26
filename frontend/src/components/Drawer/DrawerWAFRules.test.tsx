import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerWAFRules } from './DrawerWAFRules';

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>);
}

function wafListItem(overrides: { id: string; name: string; scope: string }) {
  return {
    id: overrides.id,
    name: overrides.name,
    state: 'active',
    scope: overrides.scope,
    description: '',
    rule_count: 2,
    associated_count: 0,
    tags: {},
    cost_monthly: 0,
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

// waf 一覧と waf/rules をまとめてモックする。rules を Response 生成関数にして
// エラーケースも同じ形で差し込めるようにする。
function mockFetch(list: unknown[], rules: () => Promise<Response>) {
  globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input.toString();
    if (url.includes('/waf/rules')) return rules();
    return okJson(list);
  }) as typeof fetch;
}

describe('DrawerWAFRules', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = vi.fn();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('ルールを name / priority / action / statement の 4 列で表示する', async () => {
    mockFetch([wafListItem({ id: 'acl-1', name: 'edge-acl', scope: 'REGIONAL' })], () =>
      okJson([
        { name: 'rate-limit', priority: 1, action: 'Block', statement: 'RateBased' },
        {
          name: 'common-rules',
          priority: 2,
          action: 'Override: None',
          statement: 'AWS/AWSManagedRulesCommonRuleSet',
        },
      ]),
    );

    const { container } = renderWithQC(
      <DrawerWAFRules profile="test" region="ap-northeast-1" id="acl-1" name="edge-acl" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Rules (2)');
    });
    for (const header of ['Name', 'Priority', 'Action', 'Statement']) {
      expect(container.textContent).toContain(header);
    }
    expect(container.textContent).toContain('rate-limit');
    expect(container.textContent).toContain('Block');
    expect(container.textContent).toContain('RateBased');
    expect(container.textContent).toContain('Override: None');
    expect(container.textContent).toContain('AWS/AWSManagedRulesCommonRuleSet');
  });

  it('ルール 0 件は Rules (0) の空テーブルを表示しエラー扱いにしない', async () => {
    mockFetch([wafListItem({ id: 'acl-1', name: 'edge-acl', scope: 'CLOUDFRONT' })], () =>
      okJson([]),
    );

    const { container } = renderWithQC(
      <DrawerWAFRules profile="test" region="ap-northeast-1" id="acl-1" name="edge-acl" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Rules (0)');
    });
    expect(container.querySelector('table.dt')).not.toBeNull();
    expect(container.textContent).not.toContain('Error');
  });

  it('取得エラー時はステータスとコードとメッセージを含むエラーを表示する', async () => {
    mockFetch([wafListItem({ id: 'acl-1', name: 'edge-acl', scope: 'REGIONAL' })], () =>
      Promise.resolve({
        ok: false,
        status: 403,
        statusText: 'Forbidden',
        json: async () => ({ error: 'access denied', code: 'ACCESS_DENIED' }),
      } as Response),
    );

    const { container } = renderWithQC(
      <DrawerWAFRules profile="test" region="ap-northeast-1" id="acl-1" name="edge-acl" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Error 403 (ACCESS_DENIED): access denied');
    });
    // data が無いのでテーブルは出さない (空表示と区別する)。
    expect(container.querySelector('table.dt')).toBeNull();
  });

  it('一覧キャッシュに該当行が無い間はローディング表示を出しルール取得を発火させない', async () => {
    mockFetch([wafListItem({ id: 'acl-other', name: 'other-acl', scope: 'REGIONAL' })], () =>
      okJson([]),
    );

    const { container } = renderWithQC(
      <DrawerWAFRules profile="test" region="ap-northeast-1" id="acl-1" name="edge-acl" />,
    );

    await waitFor(() => {
      expect(container.textContent).toContain('Loading…');
    });

    const fetchMock = globalThis.fetch as ReturnType<typeof vi.fn>;
    const calledRules = fetchMock.mock.calls.some(([input]) => {
      const url = typeof input === 'string' ? input : (input as URL).toString();
      return url.includes('/waf/rules');
    });
    expect(calledRules).toBe(false);
  });
});
