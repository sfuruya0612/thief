// 常駐ターミナルドックの検証。タブ切替・折りたたみ・閉じるの各操作で Terminal が
// アンマウントされないこと (= 接続が維持されること) を DOM 上の .terminal-container の
// 残存で確認する。
//
// 表示状態は getComputedStyle で判定しない。jsdom 29.1.1 の getComputedStyle は author の
// display: flex を持つ要素に hidden を付けた場合でも display を none と返し、ブラウザと
// 結果が異なるため。ここでは hidden 属性の有無だけを確認し、app.css の規則そのものと
// 実際の表示は手動確認で見る (issue 0174 の設計判断 2)。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render } from '@testing-library/react';
import { TerminalDock } from './TerminalDock';
import {
  openTerminalSession,
  resetTerminalSessionsForTest,
  type TerminalSession,
} from '../../hooks/useTerminalSessions';

class FakeWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  readonly url: string;
  binaryType = 'blob';
  readyState: number = FakeWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;

  constructor(url: string) {
    this.url = url;
  }

  send(): void {}
  close(): void {
    this.readyState = FakeWebSocket.CLOSED;
  }
}

function sessionInput(label: string): Omit<TerminalSession, 'id'> {
  return {
    kind: 'ec2',
    profile: 'test-profile',
    region: 'ap-northeast-1',
    label,
    wsUrl: `ws://127.0.0.1:8089/api/aws/profiles/test-profile/ec2/${label}/session?region=ap-northeast-1`,
  };
}

function dockHeightVar(): string {
  return document.documentElement.style.getPropertyValue('--terminal-dock-h');
}

function panes(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>('.terminal-dock-pane'));
}

describe('TerminalDock', () => {
  const originalMatchMedia = globalThis.matchMedia;

  beforeEach(() => {
    resetTerminalSessionsForTest();
    document.documentElement.style.removeProperty('--terminal-dock-h');
    vi.stubGlobal('WebSocket', FakeWebSocket);
    // xterm.js の CoreBrowserService が DPR 更新のために matchMedia を呼ぶ。jsdom は未実装のため
    // テスト用の no-op スタブを与える (DrawerTerminal.test.tsx と同じ)。
    globalThis.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
    }) as unknown as typeof globalThis.matchMedia;
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    globalThis.matchMedia = originalMatchMedia;
    vi.restoreAllMocks();
  });

  it('セッションが 0 のときはドックを描画せず --terminal-dock-h が 0px になる', () => {
    const { container } = render(<TerminalDock />);

    expect(container.querySelector('.terminal-dock')).toBeNull();
    expect(container.querySelector('.terminal-container')).toBeNull();
    expect(dockHeightVar()).toBe('0px');
  });

  it('2 セッションで両方の Terminal を同時にマウントし、非アクティブなペインだけに hidden が付く', () => {
    const { container } = render(<TerminalDock />);

    let first = '';
    let second = '';
    act(() => {
      first = openTerminalSession(sessionInput('web-01'));
      second = openTerminalSession(sessionInput('web-02'));
    });

    expect(container.querySelectorAll('.terminal-container')).toHaveLength(2);
    const [firstPane, secondPane] = panes(container);
    // 後から開いた second がアクティブ
    expect(firstPane.hasAttribute('hidden')).toBe(true);
    expect(secondPane.hasAttribute('hidden')).toBe(false);
    // .terminal-panel は author の display: flex を持つため hidden を付けない
    container.querySelectorAll('.terminal-panel').forEach((panel) => {
      expect(panel.hasAttribute('hidden')).toBe(false);
    });
    expect(first).not.toBe(second);
  });

  it('タブをクリックすると hidden の付くペインが入れ替わり、どちらの Terminal も残る', () => {
    const { container } = render(<TerminalDock />);

    act(() => {
      openTerminalSession(sessionInput('web-01'));
      openTerminalSession(sessionInput('web-02'));
    });

    const firstTab = container.querySelectorAll('.terminal-dock-tab')[0];
    fireEvent.click(firstTab);

    const [firstPane, secondPane] = panes(container);
    expect(firstPane.hasAttribute('hidden')).toBe(false);
    expect(secondPane.hasAttribute('hidden')).toBe(true);
    expect(container.querySelectorAll('.terminal-container')).toHaveLength(2);
  });

  it('折りたたみボタンで本文に hidden が付き、Terminal は残る', () => {
    const { container } = render(<TerminalDock />);

    act(() => {
      openTerminalSession(sessionInput('web-01'));
      openTerminalSession(sessionInput('web-02'));
    });

    const body = container.querySelector('.terminal-dock-body') as HTMLElement;
    expect(body.hasAttribute('hidden')).toBe(false);

    const toggle = container.querySelector('.terminal-dock-toggle') as HTMLElement;
    fireEvent.click(toggle);

    expect(
      (container.querySelector('.terminal-dock-body') as HTMLElement).hasAttribute('hidden'),
    ).toBe(true);
    expect(container.querySelectorAll('.terminal-container')).toHaveLength(2);

    fireEvent.click(container.querySelector('.terminal-dock-toggle') as HTMLElement);
    expect(
      (container.querySelector('.terminal-dock-body') as HTMLElement).hasAttribute('hidden'),
    ).toBe(false);
    expect(container.querySelectorAll('.terminal-container')).toHaveLength(2);
  });

  it('閉じるボタンで対象のセッションの Terminal だけが DOM から消える', () => {
    const { container } = render(<TerminalDock />);

    act(() => {
      openTerminalSession(sessionInput('web-01'));
      openTerminalSession(sessionInput('web-02'));
    });

    const remainingUrl = (container.querySelectorAll('.terminal-dock-tab')[1] as HTMLElement)
      .textContent;
    const closeButton = container
      .querySelectorAll('.terminal-dock-tab')[0]
      .querySelector('.terminal-dock-tab-close') as HTMLElement;
    fireEvent.click(closeButton);

    expect(container.querySelectorAll('.terminal-container')).toHaveLength(1);
    expect(container.querySelectorAll('.terminal-dock-tab')).toHaveLength(1);
    expect((container.querySelector('.terminal-dock-tab') as HTMLElement).textContent).toBe(
      remainingUrl,
    );

    fireEvent.click(container.querySelector('.terminal-dock-tab-close') as HTMLElement);
    expect(container.querySelector('.terminal-dock')).toBeNull();
    expect(container.querySelector('.terminal-container')).toBeNull();
  });

  it('ルート要素の高さと --terminal-dock-h が展開中 352px / 折りたたみ中 32px になる', () => {
    const { container } = render(<TerminalDock />);

    act(() => {
      openTerminalSession(sessionInput('web-01'));
    });

    expect((container.querySelector('.terminal-dock') as HTMLElement).style.height).toBe('352px');
    expect(dockHeightVar()).toBe('352px');

    fireEvent.click(container.querySelector('.terminal-dock-toggle') as HTMLElement);

    expect((container.querySelector('.terminal-dock') as HTMLElement).style.height).toBe('32px');
    expect(dockHeightVar()).toBe('32px');

    fireEvent.click(container.querySelector('.terminal-dock-tab-close') as HTMLElement);

    expect(container.querySelector('.terminal-dock')).toBeNull();
    expect(dockHeightVar()).toBe('0px');
  });
});
