// issue 0174: ターミナルドックが App 直下にあることで、ビュー / プロファイル /
// サービスのどれを切り替えても Terminal がアンマウントされず (= WebSocket が閉じられず)
// 接続が維持されることを検証する。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { App } from './App';
import { openTerminalSession, resetTerminalSessionsForTest } from './hooks/useTerminalSessions';

// ServicePanel が使う useResources / useCost と health check だけを差し替える
// (views/AccountView.test.tsx と同じ方法)。fetch は解決しない Promise にして
// 副作用のリクエストを止める。
const mocks = vi.hoisted(() => ({
  useResources: vi.fn(),
  useCost: vi.fn(),
  useHealthCheck: vi.fn(),
}));

vi.mock('./api/queries', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/queries')>();
  return {
    ...actual,
    useResources: mocks.useResources,
    useCost: mocks.useCost,
    useHealthCheck: mocks.useHealthCheck,
  };
});

// プロファイルタブの切替を再現する偽実装。2 プロファイルを開いた状態にし、
// activateProfile で activeProfile が切り替わる (AccountView は key={activeProfile}
// のため、切替のたびに再マウントされる)。
// issue 0175: セッションが無い状態 (分割ボタンを出さないことの検証) を作れるよう、
// hasSession をテストから切り替えられるようにする。
const profilesState = vi.hoisted(() => ({ hasSession: true }));

vi.mock('./hooks/useProfiles', async () => {
  const { useState } = await import('react');
  const PROFILES = [{ name: 'profile-a' }, { name: 'profile-b' }];
  const OPEN = ['profile-a', 'profile-b'];
  return {
    useProfiles: () => {
      const [activeProfile, setActiveProfile] = useState('profile-a');
      return {
        profiles: PROFILES,
        isLoading: false,
        isError: false,
        error: null,
        refetchProfiles: () => {},
        openProfiles: OPEN,
        activeProfile: profilesState.hasSession ? activeProfile : null,
        activateProfile: (name: string) => setActiveProfile(name),
        openProfile: () => {},
        closeProfile: () => {},
        moveProfile: () => {},
        swapProfileToVisible: () => {},
      };
    },
  };
});

// issue 0175: GCP ビューの分割ボタンはセッション (アクティブプロジェクト) が
// あるときだけ出るため、プロジェクトを持つ偽実装にする。
const gcpState = vi.hoisted(() => ({ activeProject: 'proj-a' as string | null }));

vi.mock('./hooks/useGcpProjects', () => ({
  useActiveGcpProject: () => ({
    projects: [],
    isLoading: false,
    isError: false,
    error: null,
    openProjects: gcpState.activeProject ? [gcpState.activeProject] : [],
    activeProject: gcpState.activeProject,
    activateProject: () => {},
    openProject: () => {},
    closeProject: () => {},
    moveProject: () => {},
    swapProjectToVisible: () => {},
  }),
}));

class FakeWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  static instances: FakeWebSocket[] = [];

  readonly url: string;
  binaryType = 'blob';
  readyState: number = FakeWebSocket.CONNECTING;
  closeCount = 0;
  onopen: (() => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  send(): void {}
  close(): void {
    this.closeCount += 1;
    this.readyState = FakeWebSocket.CLOSED;
  }
}

const WS_URL =
  'ws://127.0.0.1:8089/api/aws/profiles/profile-a/ec2/i-0001/session?region=ap-northeast-1';

function renderApp() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <App />
    </QueryClientProvider>,
  );
}

function clickByText(container: HTMLElement, selector: string, text: string) {
  const el = Array.from(container.querySelectorAll(selector)).find((e) =>
    (e.textContent ?? '').includes(text),
  );
  expect(el, `${selector} with text ${text}`).not.toBeUndefined();
  fireEvent.click(el!);
}

describe('App のターミナルドック', () => {
  const originalMatchMedia = globalThis.matchMedia;

  beforeEach(() => {
    localStorage.clear();
    resetTerminalSessionsForTest();
    document.documentElement.style.removeProperty('--terminal-dock-h');
    FakeWebSocket.instances = [];
    vi.clearAllMocks();
    vi.stubGlobal('WebSocket', FakeWebSocket);
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
    // xterm.js の CoreBrowserService が DPR 更新のために matchMedia を呼ぶ。jsdom は未実装。
    globalThis.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
    }) as unknown as typeof globalThis.matchMedia;
    mocks.useHealthCheck.mockReturnValue({ isSuccess: true });
    mocks.useResources.mockReturnValue({ data: [], isLoading: false, error: null });
    mocks.useCost.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    globalThis.matchMedia = originalMatchMedia;
    vi.restoreAllMocks();
  });

  it('プロファイル / ビュー / サービスを切り替えても WebSocket が閉じられず、閉じるボタンでだけ閉じる', async () => {
    const { container } = renderApp();

    act(() => {
      openTerminalSession({
        kind: 'ec2',
        profile: 'profile-a',
        region: 'ap-northeast-1',
        label: 'web-01',
        wsUrl: WS_URL,
      });
    });

    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0].url).toBe(WS_URL);
    expect(container.querySelector('.terminal-container')).not.toBeNull();

    // (1) プロファイルタブを切り替えて戻す (AccountView が key={activeProfile} で再マウントされる)
    clickByText(container, '.session-tab-name', 'profile-b');
    await waitFor(() => {
      expect(container.querySelector('.session-tab.active')?.textContent).toContain('profile-b');
    });
    clickByText(container, '.session-tab-name', 'profile-a');
    await waitFor(() => {
      expect(container.querySelector('.session-tab.active')?.textContent).toContain('profile-a');
    });
    expect(FakeWebSocket.instances[0].closeCount).toBe(0);
    expect(container.querySelector('.terminal-container')).not.toBeNull();

    // (2) トップバーのビューを aws -> gcp -> aws
    clickByText(container, '.view-switch button', 'Google Cloud');
    await waitFor(() => {
      expect(container.querySelector('.view-switch button.active')?.textContent).toBe(
        'Google Cloud',
      );
    });
    clickByText(container, '.view-switch button', 'AWS');
    await waitFor(() => {
      expect(container.querySelector('.view-switch button.active')?.textContent).toBe('AWS');
    });
    expect(FakeWebSocket.instances[0].closeCount).toBe(0);
    expect(container.querySelector('.terminal-container')).not.toBeNull();

    // (3) サイドバーでサービスを ec2 -> ssm (Parameter Store)
    clickByText(container, '.nav-item', 'Parameter Store');
    await waitFor(() => {
      expect(container.querySelector('.nav-item.active')?.textContent).toContain('Parameter Store');
    });
    expect(FakeWebSocket.instances[0].closeCount).toBe(0);
    expect(container.querySelector('.terminal-container')).not.toBeNull();

    // セッションを終了させる UI 操作はドックのタブの閉じるボタンだけ
    fireEvent.click(container.querySelector('.terminal-dock-tab-close')!);

    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0].closeCount).toBe(1);
    expect(container.querySelector('.terminal-container')).toBeNull();
    expect(container.querySelector('.terminal-dock')).toBeNull();
  });

  it('セッションが無いときはドックを描画しない', () => {
    const { container } = renderApp();

    expect(container.querySelector('.terminal-dock')).toBeNull();
    expect(FakeWebSocket.instances).toHaveLength(0);
  });
});

// issue 0175: TopBar の分割ボタンで 2 ペインにし、ペインの状態は App が持つため
// プロファイル / ビューの切り替えをまたいで残る。
describe('App の分割表示', () => {
  const originalMatchMedia = globalThis.matchMedia;

  beforeEach(() => {
    localStorage.clear();
    resetTerminalSessionsForTest();
    profilesState.hasSession = true;
    gcpState.activeProject = 'proj-a';
    vi.clearAllMocks();
    globalThis.fetch = vi.fn().mockImplementation(() => new Promise(() => {}));
    globalThis.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
    }) as unknown as typeof globalThis.matchMedia;
    mocks.useHealthCheck.mockReturnValue({ isSuccess: true });
    mocks.useResources.mockReturnValue({ data: [], isLoading: false, error: null });
    mocks.useCost.mockReturnValue({ data: [], isLoading: false, error: null });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    globalThis.matchMedia = originalMatchMedia;
    vi.restoreAllMocks();
  });

  // ec2Columns が参照するフィールドを持つ正規化後の行 (views/AccountView.test.tsx と同じ形)
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

  // ja ロケールの分割ボタン (title は topbar ネームスペースの split)
  function splitButton(container: HTMLElement): HTMLButtonElement | null {
    return container.querySelector('.topbar button[title="ペインを分割"]');
  }

  function paneEls(container: HTMLElement): HTMLElement[] {
    return Array.from(container.querySelectorAll('.body > .pane'));
  }

  async function switchView(container: HTMLElement, label: string) {
    clickByText(container, '.view-switch button', label);
    await waitFor(() => {
      expect(container.querySelector('.view-switch button.active')?.textContent).toBe(label);
    });
  }

  it('分割ボタンは AWS と Google Cloud のビューにだけ出る', async () => {
    const { container } = renderApp();
    expect(splitButton(container)).not.toBeNull();

    await switchView(container, 'Google Cloud');
    expect(splitButton(container)).not.toBeNull();

    await switchView(container, 'Datadog');
    expect(splitButton(container)).toBeNull();

    await switchView(container, 'TiDB');
    expect(splitButton(container)).toBeNull();
  });

  it('セッションが無いビューには分割ボタンを出さない', async () => {
    profilesState.hasSession = false;
    gcpState.activeProject = null;

    const { container } = renderApp();
    expect(splitButton(container)).toBeNull();

    await switchView(container, 'Google Cloud');
    expect(splitButton(container)).toBeNull();
  });

  it('分割ボタンで 2 ペインになり、もう一度押すとフォーカスしていないペインが閉じる', () => {
    const { container } = renderApp();

    fireEvent.click(splitButton(container)!);
    expect(container.querySelector('.body')!.classList.contains('split')).toBe(true);
    expect(paneEls(container)).toHaveLength(2);
    expect(paneEls(container)[0].querySelector('h1')?.textContent).toBe('EC2');
    expect(splitButton(container)!.getAttribute('aria-pressed')).toBe('true');

    // 1 つ目のペインへフォーカスを移してから押すと、残るのはフォーカス中のペイン
    fireEvent.pointerDown(paneEls(container)[0]);
    fireEvent.click(splitButton(container)!);

    expect(container.querySelector('.body')!.classList.contains('split')).toBe(false);
    // 1 ペインでも .pane (display: contents) で包む。中身はフォーカスしていた EC2 が残る
    const wrapper = container.querySelector('.body > .pane') as HTMLElement;
    expect(wrapper.classList.contains('single')).toBe(true);
    expect(container.querySelector('.body > .main')).toBeNull();
    expect(wrapper.querySelector('.main h1')?.textContent).toBe('EC2');
    expect(splitButton(container)!.getAttribute('aria-pressed')).toBe('false');
  });

  it('TopBar の分割ボタンで右のペインを閉じても、残った左のペインの選択リソースと Drawer が保たれる', () => {
    mocks.useResources.mockReturnValue({ data: [EC2_ROW], isLoading: false, error: null });
    const { container } = renderApp();
    fireEvent.click(splitButton(container)!);

    // 左のペイン (EC2) で行を選び、Drawer のタブを変えてからフォーカスを左へ置く
    const left = paneEls(container)[0];
    fireEvent.pointerDown(left);
    fireEvent.click(left.querySelector('tbody tr')!);
    const tagsTab = Array.from(left.querySelectorAll('.dtab')).find(
      (el) => el.textContent === 'Tags',
    )!;
    fireEvent.click(tagsTab);
    expect(left.querySelector('.drawer')?.classList.contains('open')).toBe(true);

    // 2 ペインで分割ボタンを押すと、フォーカスしていない右のペインが閉じる
    fireEvent.click(splitButton(container)!);

    expect(container.querySelector('.body')!.classList.contains('split')).toBe(false);
    const main = container.querySelector('.body > .pane.single > .main') as HTMLElement;
    expect(main.querySelector('h1')?.textContent).toBe('EC2');
    expect(main.querySelector('.drawer')?.classList.contains('open')).toBe(true);
    expect(main.querySelector('.drawer .id')?.textContent).toBe(EC2_ROW.id);
    expect(main.querySelector('.dtab.active')?.textContent).toBe('Tags');
  });

  it('Google Cloud のビューでも分割ボタンで 2 ペインになり、もう一度押すとフォーカスしていないペインが閉じる', async () => {
    const { container } = renderApp();
    await switchView(container, 'Google Cloud');

    fireEvent.click(splitButton(container)!);
    expect(container.querySelector('.body')!.classList.contains('split')).toBe(true);
    expect(paneEls(container)).toHaveLength(2);
    expect(paneEls(container)[0].querySelector('h1')?.textContent).toBe('Cloud Run');
    expect(paneEls(container)[1].querySelector('.pane-empty')).not.toBeNull();
    expect(splitButton(container)!.getAttribute('aria-pressed')).toBe('true');
    // AWS 側のペイン状態は変わらない (ビューごとに別の状態を持つ)
    await switchView(container, 'AWS');
    expect(container.querySelector('.body')!.classList.contains('split')).toBe(false);
    await switchView(container, 'Google Cloud');

    // 1 つ目のペインへフォーカスを移してから押すと、残るのはフォーカス中の Cloud Run
    fireEvent.pointerDown(paneEls(container)[0]);
    fireEvent.click(splitButton(container)!);

    expect(container.querySelector('.body')!.classList.contains('split')).toBe(false);
    const wrapper = container.querySelector('.body > .pane') as HTMLElement;
    expect(wrapper.classList.contains('single')).toBe(true);
    expect(wrapper.querySelector('.main h1')?.textContent).toBe('Cloud Run');
    expect(splitButton(container)!.getAttribute('aria-pressed')).toBe('false');
  });

  it('プロファイルを切り替えても分割と各ペインのサービスが残る', async () => {
    const { container } = renderApp();
    fireEvent.click(splitButton(container)!);
    clickByText(container, '.nav-item', 'Parameter Store');
    await waitFor(() => {
      expect(paneEls(container)[1].querySelector('h1')?.textContent).toBe('Parameter Store');
    });

    clickByText(container, '.session-tab-name', 'profile-b');
    await waitFor(() => {
      expect(container.querySelector('.session-tab.active')?.textContent).toContain('profile-b');
    });

    expect(container.querySelector('.body')!.classList.contains('split')).toBe(true);
    expect(paneEls(container)).toHaveLength(2);
    expect(paneEls(container)[0].querySelector('h1')?.textContent).toBe('EC2');
    expect(paneEls(container)[1].querySelector('h1')?.textContent).toBe('Parameter Store');
  });
});
