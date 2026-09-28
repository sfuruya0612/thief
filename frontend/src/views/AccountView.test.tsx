// ServicePanel (AccountView) のエラーバナー出し分けの検証 (issue 0073) と、
// 分割表示 (issue 0175) のペイン単位の挙動の検証。
// SSO トークン期限切れのときだけ SSOExpiredBanner を出し、それ以外の ApiError
// (403 ACCESS_DENIED 等) は ErrorBanner に落ちることを確認する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AccountView, type AccountViewProps } from './AccountView';
import { useSplitPanes } from '../hooks/useSplitPanes';
import { ApiError } from '../types/common';
import { SSO_TOKEN_EXPIRED_CODE } from '../lib/ssoError';
import { resetTweaksForTest } from '../hooks/useTweaks';
import { STORAGE_KEY } from '../lib/storage';

// ServicePanel が使う useResources / useCost だけを差し替え、他のフック
// (Sidebar の useRegions など) は実装のまま使う。fetch は解決しない Promise に
// して副作用のリクエストを止める。
const mocks = vi.hoisted(() => ({
  useResources: vi.fn(),
  useCost: vi.fn(),
}));

vi.mock('../api/queries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/queries')>();
  return {
    ...actual,
    useResources: mocks.useResources,
    useCost: mocks.useCost,
  };
});

// viewElement は検証対象の AccountView を QueryClientProvider で包んだ要素を返す。
// 初回の render と、props を変えずに再描画する rerender の両方で同じ要素を使う。
function viewElement(client: QueryClient, props: Partial<AccountViewProps> = {}) {
  return (
    <QueryClientProvider client={client}>
      <AccountView
        profile="test-profile"
        region="ap-northeast-1"
        profiles={[]}
        onRegionChange={() => {}}
        panes={{ services: ['ec2'], ids: [0], focused: 0 }}
        onSelectService={() => {}}
        onFocusPane={() => {}}
        onClosePane={() => {}}
        drawerPos="right"
        {...props}
      />
    </QueryClientProvider>
  );
}

function renderView(qc?: QueryClient) {
  const client = qc ?? new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(viewElement(client));
}

describe('AccountView のエラーバナー出し分け', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
    mocks.useCost.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  it('403 ACCESS_DENIED の ApiError では SSOExpiredBanner を出さず ErrorBanner に内容を表示する', () => {
    const err = new ApiError(
      403,
      'ACCESS_DENIED',
      'User is not authorized to perform: ec2:DescribeInstances',
    );
    mocks.useResources.mockReturnValue({ data: undefined, isLoading: false, error: err });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).not.toBeInTheDocument();
    const banner = container.querySelector('.error-banner');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent('403');
    expect(banner).toHaveTextContent('ACCESS_DENIED');
    expect(banner).toHaveTextContent('User is not authorized to perform: ec2:DescribeInstances');
  });

  it('401 SSO トークン期限切れの ApiError では SSOExpiredBanner を表示する', () => {
    const err = new ApiError(401, SSO_TOKEN_EXPIRED_CODE, 'SSO token expired');
    mocks.useResources.mockReturnValue({ data: undefined, isLoading: false, error: err });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).toBeInTheDocument();
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
  });

  it('エラーが無ければどちらのバナーも表示しない', () => {
    mocks.useResources.mockReturnValue({ data: [], isLoading: false, error: null });

    const { container } = renderView();

    expect(container.querySelector('.sso-banner')).not.toBeInTheDocument();
    expect(container.querySelector('.error-banner')).not.toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
  });
});

// issue 0175: 分割表示 (2 ペイン) のペイン単位の挙動。App と同じく useSplitPanes で
// 状態を持ち、サイドバーのクリック / ペインの pointerdown / ペインの閉じるボタンを
// AccountView の props 経由でつなぐ。
describe('AccountView の分割表示', () => {
  // ec2Columns は az を、s3Columns は createdAt を参照するため、正規化後の形で渡す
  const EC2_ROW = {
    id: 'i-0123456789abcdef0',
    name: 'web-01',
    state: 'running',
    region: 'ap-northeast-1',
    instanceType: 't3.micro',
    az: 'ap-northeast-1a',
    privateIp: '10.0.0.1',
    publicIp: '',
    vpcId: 'vpc-1',
    tags: { Env: 'prod' },
  };
  const S3_ROW = {
    id: 'my-bucket',
    name: 'my-bucket',
    state: 'active',
    region: 'ap-northeast-1',
    createdAt: '2026-01-01T00:00:00Z',
    public: false,
    encryption: 'AES256',
  };

  // App と同じ構成でペイン状態を持ち、分割の開始だけをテストから操作できるようにする。
  function SplitHost() {
    const api = useSplitPanes('ec2');
    return (
      <>
        <button onClick={api.openSplit}>open-split</button>
        <AccountView
          profile="test-profile"
          region="ap-northeast-1"
          profiles={[]}
          onRegionChange={() => {}}
          panes={api.panes}
          onSelectService={api.selectService}
          onFocusPane={api.focusPane}
          onClosePane={api.closePane}
          drawerPos="right"
        />
      </>
    );
  }

  function renderSplit() {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(
      <QueryClientProvider client={client}>
        <SplitHost />
      </QueryClientProvider>,
    );
  }

  function panes(container: HTMLElement): HTMLElement[] {
    return Array.from(container.querySelectorAll('.body > .pane'));
  }

  function openSplit() {
    fireEvent.click(screen.getByText('open-split'));
  }

  function clickNav(container: HTMLElement, text: string) {
    const item = Array.from(container.querySelectorAll('.nav-item')).find((el) =>
      el.textContent?.includes(text),
    );
    expect(item, `.nav-item with text ${text}`).not.toBeUndefined();
    fireEvent.click(item!);
  }

  function paneTitle(container: HTMLElement, index: number): string {
    return panes(container)[index].querySelector('h1')?.textContent ?? '';
  }

  function drawerOpen(container: HTMLElement, index: number): boolean {
    return panes(container)[index].querySelector('.drawer')?.classList.contains('open') ?? false;
  }

  // 両ペインにリソース行を返し、ペインごとの選択と Drawer を検証できるようにする。
  function mockRows() {
    mocks.useResources.mockImplementation((service: string) => ({
      data: service === 'ec2' ? [EC2_ROW] : [S3_ROW],
      isLoading: false,
      error: null,
    }));
  }

  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
    mocks.useCost.mockReturnValue({ data: [], isLoading: false, error: null });
    mocks.useResources.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  it('分割していないときは .body に split が付かず、ラッパーが display: contents の .pane.single になる', () => {
    const { container } = renderSplit();

    const body = container.querySelector('.body')!;
    expect(body.classList.contains('split')).toBe(false);
    expect(body.querySelector(':scope > .sidebar')).not.toBeNull();
    // 分割していないときも .pane で包む (display: contents でレイアウトに影響させない)
    const wrapper = body.querySelector(':scope > .pane') as HTMLElement;
    expect(wrapper.classList.contains('single')).toBe(true);
    expect(wrapper.classList.contains('focused')).toBe(false);
    expect(wrapper.querySelector('.pane-bar')).toBeNull();
    expect(wrapper.querySelector(':scope > .main')).not.toBeNull();
    // 1 ペインではフォーカスが常に 0 なので pointerdown でのフォーカス移動も付けない
    fireEvent.pointerDown(wrapper);
    expect(wrapper.classList.contains('focused')).toBe(false);
    // 分割していないときの Drawer は position: fixed のまま (contained を付けない)
    expect(container.querySelector('.drawer')?.classList.contains('contained')).toBe(false);
  });

  it('分割を開始すると 2 ペインになり、2 つ目のペインはサービス未選択の案内を出してフォーカスする', () => {
    const { container } = renderSplit();

    openSplit();

    const body = container.querySelector('.body')!;
    expect(body.classList.contains('split')).toBe(true);
    const els = panes(container);
    expect(els).toHaveLength(2);
    expect(paneTitle(container, 0)).toBe('EC2');
    expect(els[1].querySelector('.pane-empty')?.textContent).toBe(
      'サイドバーからサービスを選んでください',
    );
    expect(els[1].classList.contains('focused')).toBe(true);
    expect(els[0].classList.contains('focused')).toBe(false);
    // サービス未選択のペインがフォーカス中なので、強調する nav-item は無い
    expect(container.querySelector('.nav-item.active')).toBeNull();
    // ペイン 1 が表示中の EC2 に番号の印が出る
    const ec2Item = Array.from(container.querySelectorAll('.nav-item')).find((el) =>
      el.textContent?.includes('EC2'),
    );
    expect(ec2Item?.querySelector('.pane-mark')?.textContent).toBe('1');
  });

  it('サイドバーのクリックはフォーカス中のペインのサービスだけを変える', () => {
    const { container } = renderSplit();
    openSplit();

    clickNav(container, 'S3');
    expect(paneTitle(container, 1)).toBe('S3');
    expect(paneTitle(container, 0)).toBe('EC2');

    // 1 つ目のペインへフォーカスを移すと、そちらのサービスが変わる
    fireEvent.pointerDown(panes(container)[0]);
    clickNav(container, 'Lambda');
    expect(paneTitle(container, 0)).toBe('Lambda');
    expect(paneTitle(container, 1)).toBe('S3');
  });

  it('ペイン内の pointerdown でフォーカスが移り、もう一方のペインが表示中のサービスのクリックではフォーカスだけが移る', () => {
    const { container } = renderSplit();
    openSplit();

    // pane 1 = S3 にしてから pane 0 へフォーカスを移す
    clickNav(container, 'S3');
    fireEvent.pointerDown(panes(container)[0]);
    expect(panes(container)[0].classList.contains('focused')).toBe(true);
    expect(panes(container)[1].classList.contains('focused')).toBe(false);

    // pane 1 が表示中の S3 をクリック → フォーカスは pane 1 へ移り、サービスは変わらない
    clickNav(container, 'S3');
    expect(panes(container)[1].classList.contains('focused')).toBe(true);
    expect(paneTitle(container, 0)).toBe('EC2');
    expect(paneTitle(container, 1)).toBe('S3');
  });

  it('ペインごとに選択リソースと Drawer を持ち、片方の背景クリックでもう片方は閉じない', () => {
    mockRows();
    const { container } = renderSplit();
    openSplit();
    clickNav(container, 'S3');

    // pane 1 (S3) の行を選ぶ → pane 1 の Drawer だけが開く
    fireEvent.click(panes(container)[1].querySelector('tbody tr')!);
    expect(drawerOpen(container, 1)).toBe(true);
    expect(drawerOpen(container, 0)).toBe(false);

    // pane 0 (EC2) の行を選ぶ → 両方が独立に開く
    fireEvent.click(panes(container)[0].querySelector('tbody tr')!);
    expect(drawerOpen(container, 0)).toBe(true);
    expect(drawerOpen(container, 1)).toBe(true);
    // 分割中の Drawer はペインの中に収まる (contained が付く)
    expect(panes(container)[0].querySelector('.drawer')?.classList.contains('contained')).toBe(
      true,
    );
    expect(panes(container)[1].querySelector('.drawer')?.classList.contains('contained')).toBe(
      true,
    );

    // pane 1 の背景をクリックしても pane 0 の Drawer は閉じない
    fireEvent.click(panes(container)[1].querySelector('.drawer-backdrop')!);
    expect(drawerOpen(container, 1)).toBe(false);
    expect(drawerOpen(container, 0)).toBe(true);
  });

  it('ESC はフォーカス中のペインの Drawer だけを閉じる', () => {
    mockRows();
    const { container } = renderSplit();
    openSplit();
    clickNav(container, 'S3');
    fireEvent.click(panes(container)[1].querySelector('tbody tr')!);
    fireEvent.click(panes(container)[0].querySelector('tbody tr')!);
    expect(drawerOpen(container, 0)).toBe(true);
    expect(drawerOpen(container, 1)).toBe(true);

    fireEvent.pointerDown(panes(container)[0]);
    fireEvent.keyDown(document, { key: 'Escape' });

    expect(drawerOpen(container, 0)).toBe(false);
    expect(drawerOpen(container, 1)).toBe(true);
  });

  it('ペインごとにフィルタと Drawer のタブが独立する', () => {
    mockRows();
    const { container } = renderSplit();
    openSplit();
    clickNav(container, 'S3');

    // 先に両ペインの Drawer を開き、そのうえで pane 0 の一覧だけを絞り込む
    fireEvent.click(panes(container)[0].querySelector('tbody tr')!);
    fireEvent.click(panes(container)[1].querySelector('tbody tr')!);
    const filter0 = panes(container)[0].querySelector('.dt-col-filter') as HTMLInputElement;
    fireEvent.change(filter0, { target: { value: 'no-such-resource' } });
    expect(panes(container)[0].querySelector('tbody')?.textContent).toContain(
      'No resources match current filters',
    );
    expect(panes(container)[1].querySelector('tbody')?.textContent).toContain('my-bucket');

    // Drawer のタブもペインごと (pane 1 だけ Tags へ切り替える)
    const tagsTab = Array.from(panes(container)[1].querySelectorAll('.dtab')).find(
      (el) => el.textContent === 'Tags',
    )!;
    fireEvent.click(tagsTab);
    expect(panes(container)[1].querySelector('.dtab.active')?.textContent).toBe('Tags');
    expect(panes(container)[0].querySelector('.dtab.active')?.textContent).toBe('Overview');
  });

  // 分割していないときも各ペインを div.pane で包む (display: contents) ため、2 ペインから
  // 1 ペインへ戻ってもペインのコンポーネントは再マウントされない。左のペインを閉じると
  // 右のペインが添字 0 へ移るが、React の key はペインの識別子 (ids) なので状態が保たれる。
  it('左のペインを閉じても、残ったペインの選択リソース・Drawer・タブ・フィルタが保たれる', () => {
    mockRows();
    const { container } = renderSplit();
    openSplit();
    // 2 つ目のペイン (添字 1) に S3 を出し、行を選んで Drawer のタブとフィルタも変えておく
    clickNav(container, 'S3');
    fireEvent.click(panes(container)[1].querySelector('tbody tr')!);
    const tagsTab = Array.from(panes(container)[1].querySelectorAll('.dtab')).find(
      (el) => el.textContent === 'Tags',
    )!;
    fireEvent.click(tagsTab);
    const filter = panes(container)[1].querySelector('.dt-col-filter') as HTMLInputElement;
    fireEvent.change(filter, { target: { value: 'no-such-resource' } });
    expect(drawerOpen(container, 1)).toBe(true);
    expect(panes(container)[1].querySelector('.dtab.active')?.textContent).toBe('Tags');

    // フォーカスしていない pane 0 (左、EC2) を閉じる
    fireEvent.click(panes(container)[0].querySelector('.pane-bar .pane-close')!);

    // 残った pane 1 が .pane.single になり、サービス・選択リソース・Drawer が保たれる
    expect(container.querySelector('.body')!.classList.contains('split')).toBe(false);
    const wrapper = container.querySelector('.body > .pane') as HTMLElement;
    expect(wrapper.classList.contains('single')).toBe(true);
    const main = wrapper.querySelector('.main') as HTMLElement;
    expect(main.querySelector('h1')?.textContent).toBe('S3');
    expect(main.querySelector('.drawer')?.classList.contains('open')).toBe(true);
    expect(main.querySelector('.drawer .id')?.textContent).toBe(S3_ROW.id);
    // Drawer のタブと一覧のフィルタも初期化されない
    expect(main.querySelector('.dtab.active')?.textContent).toBe('Tags');
    expect(main.querySelector('tbody')?.textContent).toContain(
      'No resources match current filters',
    );
  });

  it('サービス未選択のペインを残して左のペインを閉じると 1 ペインで案内を出し、サイドバーの選択で復帰する', () => {
    const { container } = renderSplit();
    openSplit();

    // pane 1 はサービス未選択のまま、pane 0 (EC2) を閉じる
    fireEvent.click(panes(container)[0].querySelector('.pane-bar .pane-close')!);

    expect(container.querySelector('.body')!.classList.contains('split')).toBe(false);
    const wrapper = container.querySelector('.body > .pane') as HTMLElement;
    expect(wrapper.classList.contains('single')).toBe(true);
    expect(wrapper.querySelector('.pane-bar')).toBeNull();
    expect(wrapper.querySelector('.main.pane-empty')?.textContent).toBe(
      'サイドバーからサービスを選んでください',
    );
    // 強調する nav-item も、分割していないのでペイン番号の印も無い
    expect(container.querySelector('.nav-item.active')).toBeNull();
    expect(container.querySelector('.nav-item .pane-mark')).toBeNull();

    // サイドバーで選ぶと、残ったペインにそのサービスが出る
    clickNav(container, 'S3');
    const pane = container.querySelector('.body > .pane') as HTMLElement;
    expect(pane.querySelector('.pane-empty')).toBeNull();
    expect(pane.querySelector('.main h1')?.textContent).toBe('S3');
    expect(container.querySelector('.nav-item.active')?.textContent).toContain('S3');
  });
});

describe('AccountView の一覧取得に追随した時系列の再取得', () => {
  const TIMESERIES_KEY = ['aws', 'ec2', 'test-profile', 'ap-northeast-1', 'timeseries'];

  function newClient() {
    return new QueryClient({ defaultOptions: { queries: { retry: false } } });
  }

  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
    mocks.useCost.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  it('一覧の取得が成功したら同じ profile と region の時系列クエリを無効化する', () => {
    mocks.useResources.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      dataUpdatedAt: 1_700_000_000_000,
    });
    const qc = newClient();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');

    renderView(qc);

    expect(invalidate).toHaveBeenCalledWith({ queryKey: TIMESERIES_KEY });
  });

  it('一覧が未取得 (dataUpdatedAt が 0) の間は時系列を無効化しない', () => {
    mocks.useResources.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      dataUpdatedAt: 0,
    });
    const qc = newClient();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');

    renderView(qc);

    expect(invalidate).not.toHaveBeenCalledWith({ queryKey: TIMESERIES_KEY });
  });

  it('一覧が取り直されて dataUpdatedAt が進むたびに時系列を無効化する', () => {
    mocks.useResources.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      dataUpdatedAt: 1_700_000_000_000,
    });
    const qc = newClient();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');

    const { rerender } = renderView(qc);
    const before = invalidate.mock.calls.length;

    // Refresh で一覧を取り直した状況。時系列の取得は一覧より先に終わるため、
    // 一覧の更新に追随して取り直さないとその回の記録がグラフに入らない。
    mocks.useResources.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      dataUpdatedAt: 1_700_000_060_000,
    });
    rerender(viewElement(qc));

    expect(invalidate.mock.calls.length).toBeGreaterThan(before);
    expect(invalidate).toHaveBeenLastCalledWith({ queryKey: TIMESERIES_KEY });
  });

  it('一覧の取り直しが失敗して dataUpdatedAt が進まなければ時系列を無効化し直さない', () => {
    mocks.useResources.mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
      dataUpdatedAt: 1_700_000_000_000,
    });
    const qc = newClient();
    const invalidate = vi.spyOn(qc, 'invalidateQueries');

    const { rerender } = renderView(qc);
    // ServicePanel は呼び出しごとに新しい配列を組むため、参照ではなく中身で数える。
    const timeseriesCalls = () =>
      invalidate.mock.calls.filter(
        ([arg]) => JSON.stringify(arg?.queryKey) === JSON.stringify(TIMESERIES_KEY),
      ).length;
    expect(timeseriesCalls()).toBe(1);

    // 取得済みの一覧を持ったまま再取得に失敗した状況。TanStack Query は失敗した
    // 再取得で dataUpdatedAt を進めず、前回の data と error が並ぶ。backend は
    // 一覧の取得に失敗すると台数を記録しないため、時系列を取り直す必要も無い。
    mocks.useResources.mockReturnValue({
      data: [],
      isLoading: false,
      error: new ApiError(500, 'INTERNAL', 'DescribeInstances failed'),
      dataUpdatedAt: 1_700_000_000_000,
    });
    rerender(viewElement(qc));

    expect(timeseriesCalls()).toBe(1);
  });
});

describe('AccountView の一覧の上段 (Layout = workbench、issue 0212)', () => {
  const EC2_ROWS = [
    {
      id: 'i-1',
      name: 'web-01',
      state: 'running',
      region: 'ap-northeast-1',
      instanceType: 't3.micro',
      az: 'ap-northeast-1a',
      privateIp: '10.0.0.1',
      publicIp: '',
      vpcId: 'vpc-1',
      tags: { Env: 'prod' },
    },
    {
      id: 'i-2',
      name: 'web-02',
      state: 'stopped',
      region: 'ap-northeast-1',
      instanceType: 't3.micro',
      az: 'ap-northeast-1c',
      privateIp: '10.0.0.2',
      publicIp: '',
      vpcId: 'vpc-1',
      tags: { Env: 'stg' },
    },
  ];

  function setLayout(layout: 'standard' | 'workbench') {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        tweaks: {
          theme: 'light',
          density: 'compact',
          accent: 'green',
          drawerPos: 'right',
          lang: 'ja',
          layout,
        },
      }),
    );
    resetTweaksForTest();
  }

  function renderService(service: 'ec2' | 'ecs') {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    return render(viewElement(client, { panes: { services: [service], ids: [0], focused: 0 } }));
  }

  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
    mocks.useCost.mockReturnValue({ data: [], isLoading: false, error: null });
    mocks.useResources.mockReturnValue({ data: EC2_ROWS, isLoading: false, error: null });
  });

  afterEach(() => {
    localStorage.clear();
    resetTweaksForTest();
  });

  it('standard では toolbar / stats (カード) / facets の 3 段で、.panel-bar は出ない', () => {
    setLayout('standard');
    const { container } = renderService('ec2');
    const main = container.querySelector('.main')!;
    expect(main.querySelector('.panel-bar')).toBeNull();
    expect(main.querySelector(':scope > .toolbar h1')?.textContent).toBe('EC2');
    expect(main.querySelector(':scope > .stats .stat')).not.toBeNull();
    expect(main.querySelector(':scope > .facets .facet')).not.toBeNull();
    expect(main.querySelector('.stats-inline')).toBeNull();
  });

  it('workbench では上段が 1 行の .panel-bar (サービス名 + 要約 + チップ) になり、3 段は出ない', () => {
    setLayout('workbench');
    const { container } = renderService('ec2');
    const main = container.querySelector('.main')!;
    const bar = main.querySelector(':scope > .panel-bar')!;
    expect(bar).not.toBeNull();
    expect(bar.querySelector('.title h1')?.textContent).toBe('EC2');
    const summary = Array.from(bar.querySelectorAll('.stats-inline-item')).map(
      (el) => el.textContent,
    );
    expect(summary.slice(0, 3)).toEqual(['2 Resources', '1 Running', '1 Stopped']);
    expect(bar.querySelector('.facets .facet')).not.toBeNull();
    expect(main.querySelector(':scope > .toolbar')).toBeNull();
    expect(main.querySelector(':scope > .stats')).toBeNull();
    expect(main.querySelector(':scope > .facets')).toBeNull();
  });

  it('表と Drawer は両レイアウトで .main-row の中にあり、workbench で行を選ぶと docked で開く', () => {
    setLayout('standard');
    const standard = renderService('ec2');
    const row = standard.container.querySelector('.main-row');
    expect(row).not.toBeNull();
    expect(row!.querySelector(':scope > .table-wrap')).not.toBeNull();
    expect(row!.querySelector(':scope > .drawer')).not.toBeNull();
    expect(row!.querySelector(':scope > .drawer.docked')).toBeNull();
    expect(row!.classList.contains('drawer-bottom')).toBe(false);
    standard.unmount();

    setLayout('workbench');
    const { container } = renderService('ec2');
    expect(container.querySelector('.main-row > .drawer')).toBeNull();
    fireEvent.click(container.querySelector('tbody tr')!);
    const drawer = container.querySelector('.main-row > .drawer.docked.open');
    expect(drawer).not.toBeNull();
    expect(drawer!.querySelector('.dh h2')?.textContent).toContain('web-01');
    expect(container.querySelector('.drawer-backdrop')).toBeNull();
  });

  it('workbench の ECS ではタスク数のグラフが既定で閉じた details (.panel-collapsible) に入る', () => {
    setLayout('workbench');
    const { container } = renderService('ecs');
    const details = container.querySelector('details.panel-collapsible')!;
    expect(details).not.toBeNull();
    expect(details.hasAttribute('open')).toBe(false);
    expect(details.querySelector('summary')?.textContent).toBe('Tasks per cluster');

    setLayout('standard');
    const standard = renderService('ecs');
    expect(standard.container.querySelector('details.panel-collapsible')).toBeNull();
  });
});
