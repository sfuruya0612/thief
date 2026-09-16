import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import {
  TERMINAL_DOCK_DEFAULT_BODY_HEIGHT,
  clampTerminalDockBodyHeight,
  terminalDockBodyHeightRange,
  useTerminalDockHeight,
} from './useTerminalDockHeight';
import { STORAGE_KEY, type PersistedState } from '../lib/storage';

function storedHeight(): PersistedState['terminalDockHeight'] {
  const raw = localStorage.getItem(STORAGE_KEY);
  return raw === null ? undefined : (JSON.parse(raw) as PersistedState).terminalDockHeight;
}

describe('terminalDockBodyHeightRange', () => {
  it('下限は 160 で、上限はウィンドウ高さの 85% からタブバーの高さを引いた値', () => {
    expect(terminalDockBodyHeightRange(800)).toEqual({
      min: 160,
      max: Math.round(800 * 0.85) - 32,
    });
    expect(terminalDockBodyHeightRange(800).max).toBe(648);
    expect(terminalDockBodyHeightRange(1000).max).toBe(818);
  });

  it('上限が下限を下回るほど小さいウィンドウでは上限も 160 になる', () => {
    // 200 * 0.85 = 170、170 - 32 = 138 < 160
    expect(terminalDockBodyHeightRange(200)).toEqual({ min: 160, max: 160 });
    expect(terminalDockBodyHeightRange(0)).toEqual({ min: 160, max: 160 });
  });
});

describe('clampTerminalDockBodyHeight', () => {
  it('下限を下回る値は下限に丸める', () => {
    expect(clampTerminalDockBodyHeight(100, 800)).toBe(160);
  });

  it('上限を超える値は上限に丸める', () => {
    expect(clampTerminalDockBodyHeight(5000, 800)).toBe(648);
  });

  it('範囲内の値はそのまま返す', () => {
    expect(clampTerminalDockBodyHeight(468, 800)).toBe(468);
  });
});

describe('useTerminalDockHeight', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('terminalDockHeight が未設定なら既定値 320 で初期化される', () => {
    const { result } = renderHook(() => useTerminalDockHeight());

    expect(result.current.bodyHeight).toBe(320);
    expect(TERMINAL_DOCK_DEFAULT_BODY_HEIGHT).toBe(320);
  });

  it('terminalDockHeight が有限の数値でなければ既定値 320 で初期化される', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ terminalDockHeight: 'abc' }));
    expect(renderHook(() => useTerminalDockHeight()).result.current.bodyHeight).toBe(320);

    localStorage.setItem(STORAGE_KEY, JSON.stringify({ terminalDockHeight: null }));
    expect(renderHook(() => useTerminalDockHeight()).result.current.bodyHeight).toBe(320);
  });

  it('terminalDockHeight が数値ならその値で初期化される', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ terminalDockHeight: 500 }));

    expect(renderHook(() => useTerminalDockHeight()).result.current.bodyHeight).toBe(500);
  });

  it('setBodyHeight が state と localStorage の terminalDockHeight を更新する', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ sidebarWidth: 240 }));
    const { result } = renderHook(() => useTerminalDockHeight());

    act(() => {
      result.current.setBodyHeight(468);
    });

    expect(result.current.bodyHeight).toBe(468);
    expect(storedHeight()).toBe(468);
    // 他の永続化フィールドを消さない
    const raw = localStorage.getItem(STORAGE_KEY) as string;
    expect((JSON.parse(raw) as PersistedState).sidebarWidth).toBe(240);
  });
});
