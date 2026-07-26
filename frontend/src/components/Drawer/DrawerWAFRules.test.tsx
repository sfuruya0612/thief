import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { DrawerWAFRules } from './DrawerWAFRules';

function renderWithQC(ui: React.ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return { qc, ...render(<QueryClientProvider client={qc}>{ui}</QueryClientProvider>) };
}

// ルール名のセルを含む行 (tr) を探してクリックする。
function clickRow(container: HTMLElement, ruleName: string) {
  const cell = Array.from(container.querySelectorAll('td')).find(
    (td) => td.textContent === ruleName,
  );
  if (!cell) throw new Error(`row not found: ${ruleName}`);
  fireEvent.click(cell.closest('tr')!);
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

  it('行クリックで選択したルール名の見出しと整形済み JSON が現れ、選択行に selected クラスが付く', async () => {
    mockFetch([wafListItem({ id: 'acl-1', name: 'edge-acl', scope: 'REGIONAL' })], () =>
      okJson([
        {
          name: 'rate-limit',
          priority: 1,
          action: 'Block',
          statement: 'RateBased',
          rule_json: '{"Name":"rate-limit","Priority":1}',
        },
      ]),
    );

    const { container } = renderWithQC(
      <DrawerWAFRules profile="test" region="ap-northeast-1" id="acl-1" name="edge-acl" />,
    );
    await waitFor(() => expect(container.textContent).toContain('Rules (1)'));

    clickRow(container, 'rate-limit');

    await waitFor(() => {
      const pre = container.querySelector('pre.logbox');
      expect(pre).not.toBeNull();
      expect(pre!.textContent).toBe(JSON.stringify({ Name: 'rate-limit', Priority: 1 }, null, 2));
    });
    const row = container
      .querySelector('td')!
      .closest('tr')!
      .parentElement!.querySelector('tr.selected');
    expect(row?.textContent).toContain('rate-limit');
  });

  it('別の行をクリックすると詳細表示が切り替わり selected クラスも移動する', async () => {
    mockFetch([wafListItem({ id: 'acl-1', name: 'edge-acl', scope: 'REGIONAL' })], () =>
      okJson([
        {
          name: 'rate-limit',
          priority: 1,
          action: 'Block',
          statement: 'RateBased',
          rule_json: '{"Name":"rate-limit"}',
        },
        {
          name: 'common-rules',
          priority: 2,
          action: 'Override: None',
          statement: 'AWS/AWSManagedRulesCommonRuleSet',
          rule_json: '{"Name":"common-rules"}',
        },
      ]),
    );

    const { container } = renderWithQC(
      <DrawerWAFRules profile="test" region="ap-northeast-1" id="acl-1" name="edge-acl" />,
    );
    await waitFor(() => expect(container.textContent).toContain('Rules (2)'));

    clickRow(container, 'rate-limit');
    await waitFor(() => {
      expect(container.querySelector('pre.logbox')!.textContent).toBe(
        JSON.stringify({ Name: 'rate-limit' }, null, 2),
      );
    });

    clickRow(container, 'common-rules');
    await waitFor(() => {
      expect(container.querySelector('pre.logbox')!.textContent).toBe(
        JSON.stringify({ Name: 'common-rules' }, null, 2),
      );
    });

    const selected = container.querySelectorAll('tr.selected');
    expect(selected).toHaveLength(1);
    expect(selected[0].textContent).toContain('common-rules');
  });

  it('ruleJson が空文字の行を選択したとき pre は描画されずダッシュが表示される', async () => {
    mockFetch([wafListItem({ id: 'acl-1', name: 'edge-acl', scope: 'REGIONAL' })], () =>
      okJson([
        { name: 'rate-limit', priority: 1, action: 'Block', statement: 'RateBased', rule_json: '' },
      ]),
    );

    const { container } = renderWithQC(
      <DrawerWAFRules profile="test" region="ap-northeast-1" id="acl-1" name="edge-acl" />,
    );
    await waitFor(() => expect(container.textContent).toContain('Rules (1)'));

    clickRow(container, 'rate-limit');

    await waitFor(() => {
      expect(container.textContent).toContain('rate-limit');
    });
    expect(container.querySelector('pre.logbox')).toBeNull();
    const headings = Array.from(container.querySelectorAll('h3')).map((h) => h.textContent);
    expect(headings).toContain('rate-limit');
  });

  it('ruleJson が JSON として不正な行を選択したとき文字列がそのまま表示される', async () => {
    mockFetch([wafListItem({ id: 'acl-1', name: 'edge-acl', scope: 'REGIONAL' })], () =>
      okJson([
        {
          name: 'rate-limit',
          priority: 1,
          action: 'Block',
          statement: 'RateBased',
          rule_json: 'not-json{',
        },
      ]),
    );

    const { container } = renderWithQC(
      <DrawerWAFRules profile="test" region="ap-northeast-1" id="acl-1" name="edge-acl" />,
    );
    await waitFor(() => expect(container.textContent).toContain('Rules (1)'));

    clickRow(container, 'rate-limit');

    await waitFor(() => {
      expect(container.querySelector('pre.logbox')?.textContent).toBe('not-json{');
    });
  });

  it('列フィルタで選択行が表示から外れても見出しと pre は表示されたまま残る', async () => {
    mockFetch([wafListItem({ id: 'acl-1', name: 'edge-acl', scope: 'REGIONAL' })], () =>
      okJson([
        {
          name: 'rate-limit',
          priority: 1,
          action: 'Block',
          statement: 'RateBased',
          rule_json: '{"Name":"rate-limit"}',
        },
        {
          name: 'common-rules',
          priority: 2,
          action: 'Override: None',
          statement: 'AWS/AWSManagedRulesCommonRuleSet',
          rule_json: '{"Name":"common-rules"}',
        },
      ]),
    );

    const { container } = renderWithQC(
      <DrawerWAFRules profile="test" region="ap-northeast-1" id="acl-1" name="edge-acl" />,
    );
    await waitFor(() => expect(container.textContent).toContain('Rules (2)'));

    clickRow(container, 'rate-limit');
    await waitFor(() => {
      expect(container.querySelector('pre.logbox')).not.toBeNull();
    });

    const filterInput = container.querySelector('input.dt-col-filter') as HTMLInputElement | null;
    // Name 列は先頭のフィルタ入力
    expect(filterInput).not.toBeNull();
    fireEvent.change(filterInput!, { target: { value: 'common' } });

    await waitFor(() => {
      const tbody = container.querySelector('table.dt tbody')!;
      expect(tbody.textContent).not.toContain('rate-limit'); // 表に絞り込みが効いている
    });
    expect(container.querySelector('pre.logbox')).not.toBeNull();
    const headings = Array.from(container.querySelectorAll('h3')).map((h) => h.textContent);
    expect(headings).toContain('rate-limit');
  });

  it('選択中のルール名が再取得後の一覧から消えたとき pre が消えどの行にも selected クラスが付かない', async () => {
    let call = 0;
    globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/waf/rules')) {
        call += 1;
        if (call === 1) {
          return okJson([
            {
              name: 'rate-limit',
              priority: 1,
              action: 'Block',
              statement: 'RateBased',
              rule_json: '{"Name":"rate-limit"}',
            },
          ]);
        }
        return okJson([
          {
            name: 'common-rules',
            priority: 2,
            action: 'Override: None',
            statement: 'AWS/AWSManagedRulesCommonRuleSet',
            rule_json: '{"Name":"common-rules"}',
          },
        ]);
      }
      return okJson([wafListItem({ id: 'acl-1', name: 'edge-acl', scope: 'REGIONAL' })]);
    }) as typeof fetch;

    const { container, qc } = renderWithQC(
      <DrawerWAFRules profile="test" region="ap-northeast-1" id="acl-1" name="edge-acl" />,
    );
    await waitFor(() => expect(container.textContent).toContain('Rules (1)'));

    clickRow(container, 'rate-limit');
    await waitFor(() => {
      expect(container.querySelector('pre.logbox')).not.toBeNull();
    });

    await qc.invalidateQueries({ queryKey: ['aws', 'waf-rules'] });

    await waitFor(() => {
      expect(container.textContent).toContain('common-rules');
      expect(container.textContent).not.toContain('rate-limit');
    });
    expect(container.querySelector('pre.logbox')).toBeNull();
    expect(container.querySelectorAll('tr.selected')).toHaveLength(0);
  });
});
