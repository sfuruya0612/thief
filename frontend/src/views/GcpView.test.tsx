// 分割表示 (issue 0175) の GcpView 側の検証。AccountView と同じ分岐
// (2 ペインの描画、フォーカス中のペインへのサイドバーの反映、pointerdown での
// フォーカス移動、ペインごとの Drawer の独立、ESC の受け取り先) を固定する。
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { GcpView, type GcpViewProps } from './GcpView';
import { useSplitPanes } from '../hooks/useSplitPanes';
import type { GcpProject } from '../types/gcp';

// GcpServicePanel が使う useGcpResources だけを差し替え、他のフックは実装のまま使う
// (views/AccountView.test.tsx と同じ方法)。fetch は解決しない Promise にして副作用の
// リクエストを止める。
const mocks = vi.hoisted(() => ({
  useGcpResources: vi.fn(),
}));

vi.mock('../api/queries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/queries')>();
  return {
    ...actual,
    useGcpResources: mocks.useGcpResources,
  };
});

const PROJECTS: GcpProject[] = [
  { id: 'proj-a', name: 'Project A', projectNumber: '1', state: 'ACTIVE', createTime: '' },
];

// cloudRunColumns が location / state 等を参照するため、正規化後の形で渡す
const CLOUD_RUN_ROW = {
  id: 'run-1',
  name: 'my-service',
  state: 'ready',
  region: 'asia-northeast1',
  kind: 'service',
  url: 'https://my-service.example.run.app',
  lastModified: '',
  createdAt: '',
  labels: {},
};
const GCS_ROW = {
  id: 'my-bucket',
  name: 'my-bucket',
  state: 'active',
  region: 'asia-northeast1',
  createdAt: '',
  public: false,
  encryption: 'AES256',
};

// App と同じ構成でペイン状態を持ち、分割の開始だけをテストから操作できるようにする。
function SplitHost() {
  const api = useSplitPanes('cloudrun');
  return (
    <>
      <button onClick={api.openSplit}>open-split</button>
      <GcpView
        activeProject="proj-a"
        projects={PROJECTS}
        panes={api.panes}
        onSelectService={api.selectService}
        onFocusPane={api.focusPane}
        onClosePane={api.closePane}
        drawerPos="right"
      />
    </>
  );
}

function renderSplit(props: Partial<GcpViewProps> = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  if (Object.keys(props).length > 0) {
    return render(
      <QueryClientProvider client={client}>
        <GcpView
          activeProject="proj-a"
          projects={PROJECTS}
          panes={{ services: ['cloudrun'], ids: [0], focused: 0 }}
          onSelectService={() => {}}
          onFocusPane={() => {}}
          onClosePane={() => {}}
          drawerPos="right"
          {...props}
        />
      </QueryClientProvider>,
    );
  }
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

// 両サービスにリソース行を返し、ペインごとの選択と Drawer を検証できるようにする。
function mockRows() {
  mocks.useGcpResources.mockImplementation((service: string) => ({
    data: service === 'cloudrun' ? [CLOUD_RUN_ROW] : [GCS_ROW],
    isLoading: false,
    error: null,
  }));
}

describe('GcpView の分割表示', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
    mocks.useGcpResources.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  it('分割していないときは .body に split が付かず、ラッパーが display: contents の .pane.single になる', () => {
    const { container } = renderSplit({ panes: { services: ['cloudrun'], ids: [0], focused: 0 } });

    const body = container.querySelector('.body')!;
    expect(body.classList.contains('split')).toBe(false);
    expect(body.querySelector(':scope > .sidebar')).not.toBeNull();
    // 分割していないときも .pane で包む (display: contents でレイアウトに影響させない)
    const wrapper = body.querySelector(':scope > .pane') as HTMLElement;
    expect(wrapper.classList.contains('single')).toBe(true);
    expect(wrapper.classList.contains('focused')).toBe(false);
    expect(wrapper.querySelector('.pane-bar')).toBeNull();
    expect(wrapper.querySelector(':scope > .main')).not.toBeNull();
    // 分割していないときはサイドバーにペイン番号の印を出さない
    expect(container.querySelector('.nav-item .pane-mark')).toBeNull();
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
    expect(paneTitle(container, 0)).toBe('Cloud Run');
    expect(els[1].querySelector('.pane-empty')?.textContent).toBe(
      'サイドバーからサービスを選んでください',
    );
    expect(els[1].classList.contains('focused')).toBe(true);
    expect(els[0].classList.contains('focused')).toBe(false);
    // サービス未選択のペインがフォーカス中なので、強調する nav-item は無い
    expect(container.querySelector('.nav-item.active')).toBeNull();
    // ペイン 1 が表示中の Cloud Run に番号の印が出る
    const cloudRunItem = Array.from(container.querySelectorAll('.nav-item')).find((el) =>
      el.textContent?.includes('Cloud Run'),
    );
    expect(cloudRunItem?.querySelector('.pane-mark')?.textContent).toBe('1');
  });

  it('サイドバーのクリックはフォーカス中のペインのサービスだけを変える', () => {
    const { container } = renderSplit();
    openSplit();

    clickNav(container, 'Cloud Storage');
    expect(paneTitle(container, 1)).toBe('Cloud Storage');
    expect(paneTitle(container, 0)).toBe('Cloud Run');
    // ペイン 2 が表示中の Cloud Storage に番号 2 の印が出る
    const storageItem = Array.from(container.querySelectorAll('.nav-item')).find((el) =>
      el.textContent?.includes('Cloud Storage'),
    );
    expect(storageItem?.querySelector('.pane-mark')?.textContent).toBe('2');

    // 1 つ目のペインへフォーカスを移すと、そちらのサービスが変わる
    fireEvent.pointerDown(panes(container)[0]);
    clickNav(container, 'IAM');
    expect(paneTitle(container, 0)).toBe('IAM');
    expect(paneTitle(container, 1)).toBe('Cloud Storage');
  });

  it('ペイン内の pointerdown でフォーカスが移り、もう一方のペインが表示中のサービスのクリックではフォーカスだけが移る', () => {
    const { container } = renderSplit();
    openSplit();

    // pane 1 = Cloud Storage にしてから pane 0 へフォーカスを移す
    clickNav(container, 'Cloud Storage');
    fireEvent.pointerDown(panes(container)[0]);
    expect(panes(container)[0].classList.contains('focused')).toBe(true);
    expect(panes(container)[1].classList.contains('focused')).toBe(false);

    // pane 1 が表示中の Cloud Storage をクリック → フォーカスは pane 1 へ、サービスは変わらない
    clickNav(container, 'Cloud Storage');
    expect(panes(container)[1].classList.contains('focused')).toBe(true);
    expect(paneTitle(container, 0)).toBe('Cloud Run');
    expect(paneTitle(container, 1)).toBe('Cloud Storage');
  });

  it('ペインごとに選択リソースと Drawer を持ち、片方の背景クリックでもう片方は閉じない', () => {
    mockRows();
    const { container } = renderSplit();
    openSplit();
    clickNav(container, 'Cloud Storage');

    // pane 1 (Cloud Storage) の行を選ぶ → pane 1 の Drawer だけが開く
    fireEvent.click(panes(container)[1].querySelector('tbody tr')!);
    expect(drawerOpen(container, 1)).toBe(true);
    expect(drawerOpen(container, 0)).toBe(false);

    // pane 0 (Cloud Run) の行を選ぶ → 両方が独立に開く
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
    clickNav(container, 'Cloud Storage');
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
    clickNav(container, 'Cloud Storage');

    // 先に両ペインの Drawer を開き、そのうえで pane 0 の一覧だけを絞り込む
    fireEvent.click(panes(container)[0].querySelector('tbody tr')!);
    fireEvent.click(panes(container)[1].querySelector('tbody tr')!);
    const filter0 = panes(container)[0].querySelector('.dt-col-filter') as HTMLInputElement;
    fireEvent.change(filter0, { target: { value: 'no-such-resource' } });
    expect(panes(container)[0].querySelector('tbody')?.textContent).toContain(
      'No resources match current filters',
    );
    expect(panes(container)[1].querySelector('tbody')?.textContent).toContain('my-bucket');

    // Drawer のタブもペインごと (pane 1 だけ Objects へ切り替える)
    const objectsTab = Array.from(panes(container)[1].querySelectorAll('.dtab')).find(
      (el) => el.textContent === 'Objects',
    )!;
    fireEvent.click(objectsTab);
    expect(panes(container)[1].querySelector('.dtab.active')?.textContent).toBe('Objects');
    expect(panes(container)[0].querySelector('.dtab.active')?.textContent).toBe('Overview');
  });

  // 分割していないときも各ペインを div.pane で包む (display: contents) ため、2 ペインから
  // 1 ペインへ戻ってもペインのコンポーネントは再マウントされない (AccountView.test.tsx と同じ)。
  it('左のペインを閉じても、残ったペインの選択リソース・Drawer・タブ・フィルタが保たれる', () => {
    mockRows();
    const { container } = renderSplit();
    openSplit();
    // 2 つ目のペイン (添字 1) に Cloud Storage を出し、行を選んで Drawer のタブとフィルタも変える
    clickNav(container, 'Cloud Storage');
    fireEvent.click(panes(container)[1].querySelector('tbody tr')!);
    const objectsTab = Array.from(panes(container)[1].querySelectorAll('.dtab')).find(
      (el) => el.textContent === 'Objects',
    )!;
    fireEvent.click(objectsTab);
    const filter = panes(container)[1].querySelector('.dt-col-filter') as HTMLInputElement;
    fireEvent.change(filter, { target: { value: 'no-such-resource' } });
    expect(drawerOpen(container, 1)).toBe(true);
    expect(panes(container)[1].querySelector('.dtab.active')?.textContent).toBe('Objects');

    // フォーカスしていない pane 0 (左、Cloud Run) を閉じる
    fireEvent.click(panes(container)[0].querySelector('.pane-bar .pane-close')!);

    expect(container.querySelector('.body')!.classList.contains('split')).toBe(false);
    const wrapper = container.querySelector('.body > .pane') as HTMLElement;
    expect(wrapper.classList.contains('single')).toBe(true);
    const main = wrapper.querySelector('.main') as HTMLElement;
    expect(main.querySelector('h1')?.textContent).toBe('Cloud Storage');
    expect(main.querySelector('.drawer')?.classList.contains('open')).toBe(true);
    expect(main.querySelector('.drawer .id')?.textContent).toBe(GCS_ROW.id);
    expect(main.querySelector('.dtab.active')?.textContent).toBe('Objects');
    expect(main.querySelector('tbody')?.textContent).toContain(
      'No resources match current filters',
    );
  });
});
