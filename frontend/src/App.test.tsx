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
        activeProfile,
        activateProfile: (name: string) => setActiveProfile(name),
        openProfile: () => {},
        closeProfile: () => {},
        moveProfile: () => {},
        swapProfileToVisible: () => {},
      };
    },
  };
});

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
