import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Drawer } from './Drawer';
import type { BaseRow } from '../../types/common';
import { resetTerminalSessionsForTest, useTerminalSessions } from '../../hooks/useTerminalSessions';

const RESOURCE: BaseRow = {
  id: 'i-0123456789abcdef0',
  name: 'test-instance',
  state: 'running',
};

function renderDrawer(props: Partial<React.ComponentProps<typeof Drawer>> = {}) {
  return render(
    <Drawer
      resource={RESOURCE}
      service="ec2"
      profile="test-profile"
      region="ap-northeast-1"
      overviewRows={[]}
      onClose={() => {}}
      {...props}
    />,
  );
}

function drawerElement(container: HTMLElement): HTMLElement {
  const el = container.querySelector('.drawer');
  expect(el).not.toBeNull();
  return el as HTMLElement;
}

describe('Drawer のサイズクランプ', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('永続化された height が現在のウィンドウ高さの 85% にクランプされる (画面全体を覆う不具合の回帰テスト)', () => {
    localStorage.setItem('cloudlens:drawerSize', JSON.stringify({ height: 900 }));
    window.innerHeight = 600;

    const { container } = renderDrawer({ position: 'bottom' });

    expect(drawerElement(container).style.height).toBe('510px'); // 600 * 0.85
  });

  it('永続化された height がウィンドウ高さの 85% 未満ならそのまま適用される', () => {
    localStorage.setItem('cloudlens:drawerSize', JSON.stringify({ height: 300 }));
    window.innerHeight = 600;

    const { container } = renderDrawer({ position: 'bottom' });

    expect(drawerElement(container).style.height).toBe('300px');
  });

  it('永続化された width が現在のウィンドウ幅の 85% にクランプされる', () => {
    localStorage.setItem('cloudlens:drawerSize', JSON.stringify({ width: 2000 }));
    window.innerWidth = 1000;

    const { container } = renderDrawer({ position: 'right' });

    expect(drawerElement(container).style.width).toBe('850px'); // 1000 * 0.85
  });

  it('永続値がない場合は inline の height/width を設定しない (CSS デフォルトに任せる)', () => {
    const { container } = renderDrawer({ position: 'bottom' });

    const drawer = drawerElement(container);
    expect(drawer.style.height).toBe('');
    expect(drawer.style.width).toBe('');
  });
});

describe('Drawer の ESC キー', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('open 中に Escape で onClose が呼ばれる', () => {
    const onClose = vi.fn();
    renderDrawer({ onClose });

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('Escape 以外のキーでは onClose が呼ばれない', () => {
    const onClose = vi.fn();
    renderDrawer({ onClose });

    fireEvent.keyDown(document, { key: 'Enter' });

    expect(onClose).not.toHaveBeenCalled();
  });

  it('閉じている (resource が null) 場合は Escape でも onClose が呼ばれない', () => {
    const onClose = vi.fn();
    renderDrawer({ resource: null, onClose });

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).not.toHaveBeenCalled();
  });
});

// issue 0175: 分割表示中は Drawer をペインの中に収める (contained) ため、配置と
// リサイズの基準をビューポートから包含ブロック (ペインの .main) へ切り替える。
describe('Drawer の分割表示向けの contained / closeOnEscape', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  // jsdom は offsetParent を実装しない (常に null) ため、包含ブロックを差し込んで
  // リサイズがペインの矩形を基準にすることを検証する。
  function stubContainingBlock(el: HTMLElement, rect: DOMRect) {
    const block = document.createElement('div');
    block.getBoundingClientRect = () => rect;
    Object.defineProperty(el, 'offsetParent', { value: block, configurable: true });
  }

  function rect(left: number, top: number, width: number, height: number): DOMRect {
    return {
      left,
      top,
      width,
      height,
      right: left + width,
      bottom: top + height,
      x: left,
      y: top,
      toJSON: () => ({}),
    } as DOMRect;
  }

  it('contained を渡すと .drawer と .drawer-backdrop に contained が付き、既定値では付かない', () => {
    const contained = renderDrawer({ contained: true });
    expect(drawerElement(contained.container).classList.contains('contained')).toBe(true);
    expect(
      contained.container.querySelector('.drawer-backdrop')?.classList.contains('contained'),
    ).toBe(true);

    const plain = renderDrawer();
    expect(drawerElement(plain.container).classList.contains('contained')).toBe(false);
    expect(plain.container.querySelector('.drawer-backdrop')?.classList.contains('contained')).toBe(
      false,
    );
  });

  it('closeOnEscape=false のとき ESC で onClose を呼ばない (フォーカスしていないペイン用)', () => {
    const onClose = vi.fn();
    renderDrawer({ closeOnEscape: false, onClose });

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(onClose).not.toHaveBeenCalled();
  });

  it('contained の右配置は包含ブロックの矩形を基準に幅を計算する', () => {
    const { container } = renderDrawer({ contained: true, position: 'right' });
    const drawer = drawerElement(container);
    stubContainingBlock(drawer, rect(100, 50, 1000, 800));

    fireEvent.pointerDown(drawer.querySelector('.resize-handle')!);
    fireEvent(document, new MouseEvent('pointermove', { clientX: 300 }));
    fireEvent(document, new MouseEvent('pointerup'));

    // 1000 - (300 - 100) - 8。ウィンドウ幅 (jsdom 既定の 1024) 基準なら 716 になる
    expect(drawer.style.width).toBe('792px');
  });

  it('contained の右配置の上限は包含ブロック幅の 85% になる', () => {
    const { container } = renderDrawer({ contained: true, position: 'right' });
    const drawer = drawerElement(container);
    stubContainingBlock(drawer, rect(0, 0, 400, 800));

    fireEvent.pointerDown(drawer.querySelector('.resize-handle')!);
    fireEvent(document, new MouseEvent('pointermove', { clientX: 1000 }));
    fireEvent(document, new MouseEvent('pointerup'));

    expect(drawer.style.width).toBe('340px'); // 400 * 0.85
  });

  it('contained の下配置は包含ブロックの矩形を基準に高さを計算する', () => {
    const { container } = renderDrawer({ contained: true, position: 'bottom' });
    const drawer = drawerElement(container);
    stubContainingBlock(drawer, rect(0, 50, 800, 600));

    fireEvent.pointerDown(drawer.querySelector('.resize-handle')!);
    fireEvent(document, new MouseEvent('pointermove', { clientY: 200 }));
    fireEvent(document, new MouseEvent('pointerup'));

    // 600 - (200 - 50) - 8。ウィンドウ高さ (jsdom 既定の 768) 基準なら 610 になる
    expect(drawer.style.height).toBe('442px');
  });

  it('contained の下配置の上限は包含ブロック高さの 85% になる', () => {
    const { container } = renderDrawer({ contained: true, position: 'bottom' });
    const drawer = drawerElement(container);
    stubContainingBlock(drawer, rect(0, 0, 800, 200));

    fireEvent.pointerDown(drawer.querySelector('.resize-handle')!);
    fireEvent(document, new MouseEvent('pointermove', { clientY: 0 }));
    fireEvent(document, new MouseEvent('pointerup'));

    expect(drawer.style.height).toBe('170px'); // 200 * 0.85
  });
});

describe('Drawer の開閉クラスと transform の定義元', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  // 閉じ位置と開き位置は app.css の .drawer / .drawer.open / .drawer.pos-bottom /
  // .drawer.pos-bottom.open だけで定義する。inline の transform を残すと、下配置の閉じ位置が
  // 参照する --terminal-dock-h の加算を CSS 側だけ直す余地が生まれる (issue 0174 の reopen)。
  it('閉じた下配置の Drawer は pos-bottom を持ち open を持たず、inline の transform を設定しない', () => {
    const { container } = renderDrawer({ resource: null, position: 'bottom' });

    const drawer = drawerElement(container);
    expect(drawer.classList.contains('pos-bottom')).toBe(true);
    expect(drawer.classList.contains('open')).toBe(false);
    expect(drawer.style.transform).toBe('');
  });

  it('閉じた右配置の Drawer は pos-bottom も open も持たず、inline の transform を設定しない', () => {
    const { container } = renderDrawer({ resource: null, position: 'right' });

    const drawer = drawerElement(container);
    expect(drawer.classList.contains('pos-bottom')).toBe(false);
    expect(drawer.classList.contains('open')).toBe(false);
    expect(drawer.style.transform).toBe('');
  });

  it('開いた Drawer は open を持ち、inline の transform を設定しない', () => {
    const { container } = renderDrawer({ position: 'bottom' });

    const drawer = drawerElement(container);
    expect(drawer.classList.contains('open')).toBe(true);
    expect(drawer.style.transform).toBe('');
  });
});

function tabLabels(container: HTMLElement): string[] {
  return Array.from(container.querySelectorAll('.dtab')).map((el) => el.textContent ?? '');
}

describe('Drawer のタブ構成', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('rds は Instance Parameters と Cluster Parameters の 2 タブに分かれ Parameters タブが無い', () => {
    const { container } = renderDrawer({ service: 'rds' });

    const labels = tabLabels(container);
    expect(labels).toEqual(['Overview', 'Instance Parameters', 'Cluster Parameters', 'Tags']);
    expect(labels).not.toContain('Parameters');
  });

  it('cache は現状の Parameters タブのまま変わらない', () => {
    const { container } = renderDrawer({ service: 'cache' });

    expect(tabLabels(container)).toEqual(['Overview', 'Parameters', 'Tags']);
  });

  it('cloudfront は Overview, Behaviors, Tags の 3 タブになる', () => {
    const { container } = renderDrawer({ service: 'cloudfront' });

    expect(tabLabels(container)).toEqual(['Overview', 'Behaviors', 'Tags']);
  });
});

describe('Drawer の Tags タブと tagsFetchFailed の連携', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  function switchToTagsTab(container: HTMLElement) {
    const tab = Array.from(container.querySelectorAll('.dtab')).find(
      (el) => el.textContent === 'Tags',
    );
    expect(tab).not.toBeUndefined();
    fireEvent.click(tab!);
  }

  it('tagsFetchFailed が true のとき Tags タブに取得失敗の見出しと警告アイコンを表示する', () => {
    const { container } = renderDrawer({
      service: 'waf',
      resource: { ...RESOURCE, tags: {}, tagsFetchFailed: true },
    });

    switchToTagsTab(container);

    expect(container.textContent).toContain('Tags (取得失敗)');
    expect(container.querySelector('.fetch-failed-warning')).not.toBeNull();
  });

  it('tagsFetchFailed が false のとき Tags タブは通常の件数見出しで警告アイコンを表示しない', () => {
    const { container } = renderDrawer({
      service: 'waf',
      resource: { ...RESOURCE, tags: { env: 'prod' }, tagsFetchFailed: false },
    });

    switchToTagsTab(container);

    expect(container.textContent).toContain('Tags (1)');
    expect(container.querySelector('.fetch-failed-warning')).toBeNull();
  });
});

describe('Drawer の CloudFront Behaviors タブ', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('Behaviors タブでビヘイビアの pathPattern が表示される', async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        statusText: 'OK',
        json: async () => [
          {
            id: 'i-0123456789abcdef0',
            name: 'test-instance',
            state: 'deployed',
            domain_name: 'd123.cloudfront.net',
            aliases: null,
            origins: null,
            behaviors: [
              {
                path_pattern: '/images/*',
                target_origin_id: 'origin-1',
                viewer_protocol_policy: 'https-only',
                allowed_methods: ['GET', 'HEAD'],
                compress: true,
                is_default: false,
              },
            ],
            enabled: true,
            price_class: 'PriceClass_All',
            cost_monthly: 0,
          },
        ],
      } as Response),
    ) as typeof fetch;

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { container } = render(
      <QueryClientProvider client={qc}>
        <Drawer
          resource={RESOURCE}
          service="cloudfront"
          profile="test-profile"
          region="ap-northeast-1"
          overviewRows={[]}
          onClose={() => {}}
        />
      </QueryClientProvider>,
    );

    const tab = Array.from(container.querySelectorAll('.dtab')).find(
      (el) => el.textContent === 'Behaviors',
    );
    expect(tab).not.toBeUndefined();
    fireEvent.click(tab!);

    await waitFor(() => {
      expect(container.textContent).toContain('/images/*');
    });
  });
});

describe('Drawer の RDS パラメータタブのエラー分離', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('Cluster 側の取得がエラーでも Instance Parameters タブにパラメータが表示される', async () => {
    const instance = {
      id: 'db-1',
      name: 'db-1',
      state: 'available',
      engine: 'aurora-mysql',
      engine_version: '8.0.mysql_aurora.3.04.0',
      class: 'db.r6g.large',
      multi_az: false,
      endpoint: 'db.example.com',
      port: 3306,
      vpc_id: 'vpc-1',
      parameter_groups: ['default.aurora-mysql8.0'],
      cluster_id: 'aurora-cluster-1',
      tags: {},
      cost_monthly: 0,
      launch_time: '2026-01-01T00:00:00Z',
    };
    globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/rds/cluster-parameters')) {
        return Promise.resolve({
          ok: false,
          status: 403,
          statusText: 'Forbidden',
          json: async () => ({ error: 'access denied', code: 'ACCESS_DENIED' }),
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

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { container } = render(
      <QueryClientProvider client={qc}>
        <Drawer
          resource={{ id: 'db-1', name: 'db-1', state: 'available' }}
          service="rds"
          profile="test-profile"
          region="ap-northeast-1"
          overviewRows={[]}
          onClose={() => {}}
        />
      </QueryClientProvider>,
    );

    const clickTab = (label: string) => {
      const tab = Array.from(container.querySelectorAll('.dtab')).find(
        (el) => el.textContent === label,
      );
      expect(tab).not.toBeUndefined();
      fireEvent.click(tab!);
    };

    // 先に Cluster 側をエラーにしてから Instance 側へ移る (エラー分離の検証)。
    clickTab('Cluster Parameters');
    await waitFor(() => {
      expect(container.textContent).toContain('Error 403 (ACCESS_DENIED): access denied');
    });

    clickTab('Instance Parameters');
    await waitFor(() => {
      expect(container.textContent).toContain('max_connections');
    });
  });
});

// issue 0174: Tasks タブの Exec は Terminal タブへ切り替えず、常駐ターミナルドックへ
// 直接セッションを開く (Drawer 側に pendingExecTarget を持たない)。
describe('Drawer の ECS Tasks タブの Exec', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    localStorage.clear();
    resetTerminalSessionsForTest();
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      statusText: 'OK',
      json: async () => [
        {
          arn: 'arn:aws:ecs:ap-northeast-1:123:task/my-cluster/abc',
          group: 'service:my-svc',
          last_status: 'RUNNING',
          desired_status: 'RUNNING',
          launch_type: 'FARGATE',
          enable_execute_command: true,
          container_names: ['app'],
          cpu: '256',
          memory: '512',
          started_at: '2026-07-08T00:00:00Z',
          stopped_at: '',
          stopped_reason: '',
          containers: [
            {
              name: 'app',
              image: 'app:latest',
              last_status: 'RUNNING',
              health_status: 'HEALTHY',
              reason: '',
              runtime_id: 'runtime-app',
              cpu: '',
              memory: '',
              memory_reservation: '',
              exec_enabled: true,
            },
          ],
        },
      ],
    } as Response);
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('Exec ボタンでストアに kind: ecs のセッションが追加され、タブは Terminal へ切り替わらない', async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { container } = render(
      <QueryClientProvider client={qc}>
        <Drawer
          resource={{ id: 'my-cluster', name: 'my-cluster', state: 'ACTIVE' }}
          service="ecs"
          profile="test-profile"
          region="ap-northeast-1"
          overviewRows={[]}
          onClose={() => {}}
        />
      </QueryClientProvider>,
    );

    const tasksTab = Array.from(container.querySelectorAll('.dtab')).find(
      (el) => el.textContent === 'Tasks',
    );
    fireEvent.click(tasksTab!);

    await waitFor(() => {
      expect(container.querySelector('td .primary.truncate')).not.toBeNull();
    });
    fireEvent.click(container.querySelector('td .primary.truncate')!);

    await waitFor(() => {
      expect(
        Array.from(container.querySelectorAll('button')).find((b) => b.textContent === 'Exec'),
      ).not.toBeUndefined();
    });
    const execButton = Array.from(container.querySelectorAll('button')).find(
      (b) => b.textContent === 'Exec',
    )!;
    fireEvent.click(execButton);

    const state = renderHook(() => useTerminalSessions()).result.current;
    expect(state.tabs.open).toHaveLength(1);
    const session = state.sessions[state.tabs.active];
    expect(session.kind).toBe('ecs');
    expect(session.label).toBe('my-cluster / abc / app');
    expect(session.wsUrl).toContain('/ecs/my-cluster/tasks/abc/exec');

    // Drawer のタブは Tasks のまま (Terminal へ切り替わらない)
    const activeTab = container.querySelector('.dtab.active');
    expect(activeTab?.textContent).toBe('Tasks');
  });
});
