// Terminal 単体の検証。WebSocket の生成とクローズ、非表示時の fit() 抑止 (issue 0174 の
// ResizeObserver ガード)、active prop の false -> true でのフィットとフォーカスを見る。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from '@testing-library/react';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal as XTerm } from '@xterm/xterm';
import { Terminal } from './Terminal';

const WS_URL = 'ws://127.0.0.1:8089/api/aws/profiles/p/ec2/i-0001/session?region=ap-northeast-1';

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

// 発火させられる ResizeObserver スタブ (jsdom は ResizeObserver を持たない)
class ControllableResizeObserver {
  static callbacks: Array<() => void> = [];

  constructor(callback: () => void) {
    ControllableResizeObserver.callbacks.push(callback);
  }

  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}

  static trigger(): void {
    ControllableResizeObserver.callbacks.forEach((cb) => cb());
  }
}

describe('Terminal', () => {
  const originalMatchMedia = globalThis.matchMedia;
  let fitSpy: ReturnType<typeof vi.spyOn>;
  let focusSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    FakeWebSocket.instances = [];
    ControllableResizeObserver.callbacks = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
    vi.stubGlobal('ResizeObserver', ControllableResizeObserver);
    // xterm.js の CoreBrowserService が DPR 更新のために matchMedia を呼ぶ。jsdom は未実装のため
    // テスト用の no-op スタブを与える (DrawerTerminal.test.tsx と同じ)。
    globalThis.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
    }) as unknown as typeof globalThis.matchMedia;
    fitSpy = vi.spyOn(FitAddon.prototype, 'fit').mockImplementation(() => {});
    focusSpy = vi.spyOn(XTerm.prototype, 'focus').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    globalThis.matchMedia = originalMatchMedia;
    vi.restoreAllMocks();
  });

  it('マウント時に prop の wsUrl で WebSocket を生成し、アンマウント時に close() を 1 回呼ぶ', () => {
    const { unmount } = render(<Terminal wsUrl={WS_URL} />);

    expect(FakeWebSocket.instances).toHaveLength(1);
    expect(FakeWebSocket.instances[0].url).toBe(WS_URL);
    expect(FakeWebSocket.instances[0].closeCount).toBe(0);

    unmount();

    expect(FakeWebSocket.instances[0].closeCount).toBe(1);
  });

  it('コンテナの寸法が 0 のときは ResizeObserver の発火で fit() を呼ばない', () => {
    const { container } = render(<Terminal wsUrl={WS_URL} />);

    const el = container.querySelector('.terminal-container') as HTMLElement;
    expect(el.clientWidth).toBe(0);
    expect(el.clientHeight).toBe(0);
    fitSpy.mockClear();

    ControllableResizeObserver.trigger();

    expect(fitSpy).not.toHaveBeenCalled();
  });

  it('コンテナに寸法があるときは ResizeObserver の発火で fit() を呼ぶ', () => {
    const { container } = render(<Terminal wsUrl={WS_URL} />);

    const el = container.querySelector('.terminal-container') as HTMLElement;
    Object.defineProperty(el, 'clientWidth', { value: 640, configurable: true });
    Object.defineProperty(el, 'clientHeight', { value: 320, configurable: true });
    fitSpy.mockClear();

    ControllableResizeObserver.trigger();

    expect(fitSpy).toHaveBeenCalledTimes(1);
  });

  it('active が false から true に変わると fit() と focus() を呼ぶ', () => {
    const { rerender } = render(<Terminal wsUrl={WS_URL} active={false} />);

    fitSpy.mockClear();
    focusSpy.mockClear();

    rerender(<Terminal wsUrl={WS_URL} active={true} />);

    expect(fitSpy).toHaveBeenCalledTimes(1);
    expect(focusSpy).toHaveBeenCalledTimes(1);
  });

  it('active が true のまま再描画されても fit() と focus() を呼び直さない', () => {
    const { rerender } = render(<Terminal wsUrl={WS_URL} active={true} />);

    fitSpy.mockClear();
    focusSpy.mockClear();

    rerender(<Terminal wsUrl={WS_URL} active={true} />);

    expect(fitSpy).not.toHaveBeenCalled();
    expect(focusSpy).not.toHaveBeenCalled();
  });
});
