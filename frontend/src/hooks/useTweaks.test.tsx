import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it } from 'vitest';
import { DEFAULT_TWEAKS, resetTweaksForTest, useTweaks } from './useTweaks';
import { STORAGE_KEY, type PersistedState } from '../lib/storage';

function loadStoredTweaks(): PersistedState['tweaks'] {
  const raw = localStorage.getItem(STORAGE_KEY);
  return raw === null ? undefined : (JSON.parse(raw) as PersistedState).tweaks;
}

describe('useTweaks', () => {
  beforeEach(() => {
    localStorage.clear();
    resetTweaksForTest();
  });

  it('デフォルト値で初期化される', () => {
    const { result } = renderHook(() => useTweaks());
    expect(result.current.tweaks).toEqual(DEFAULT_TWEAKS);
  });

  it('localStorage の永続値をデフォルトにマージして初期化される', () => {
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({ tweaks: { ...DEFAULT_TWEAKS, drawerPos: 'right' } }),
    );
    const { result } = renderHook(() => useTweaks());
    expect(result.current.tweaks.drawerPos).toBe('right');
    expect(result.current.tweaks.theme).toBe(DEFAULT_TWEAKS.theme);
  });

  it('別インスタンスの update が全インスタンスへ即時反映される (Detail panel 切り替えの回帰テスト)', () => {
    // App 側と TweaksPanel 側の 2 インスタンスを再現する
    const app = renderHook(() => useTweaks());
    const panel = renderHook(() => useTweaks());

    expect(app.result.current.tweaks.drawerPos).toBe('bottom');

    act(() => {
      panel.result.current.update({ drawerPos: 'right' });
    });

    expect(app.result.current.tweaks.drawerPos).toBe('right');
    expect(panel.result.current.tweaks.drawerPos).toBe('right');
  });

  it('update が document.documentElement の data-* 属性へ反映される', () => {
    const { result } = renderHook(() => useTweaks());

    act(() => {
      result.current.update({ theme: 'dark', accent: 'purple' });
    });

    const root = document.documentElement;
    expect(root.getAttribute('data-theme')).toBe('dark');
    expect(root.getAttribute('data-accent')).toBe('purple');
    expect(root.getAttribute('data-density')).toBe(DEFAULT_TWEAKS.density);
  });

  it('update が localStorage に永続化され、再初期化後に読み戻される', () => {
    const first = renderHook(() => useTweaks());

    act(() => {
      first.result.current.update({ drawerPos: 'right', theme: 'dark' });
    });

    expect(loadStoredTweaks()).toMatchObject({ drawerPos: 'right', theme: 'dark' });

    // リロード相当: 共有ストアを破棄して localStorage から初期化し直す
    first.unmount();
    resetTweaksForTest();
    const second = renderHook(() => useTweaks());
    expect(second.result.current.tweaks.drawerPos).toBe('right');
    expect(second.result.current.tweaks.theme).toBe('dark');
  });

  it('永続化は tweaks 以外のフィールドを保持したままマージする', () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ region: 'ap-northeast-1' }));
    const { result } = renderHook(() => useTweaks());

    act(() => {
      result.current.update({ theme: 'dark' });
    });

    const persisted = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') as PersistedState;
    expect(persisted.region).toBe('ap-northeast-1');
    expect(persisted.tweaks?.theme).toBe('dark');
  });
});
