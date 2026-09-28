import { useCallback, useEffect, useSyncExternalStore } from 'react';
import type { Layout, Tweaks } from '../types/common';
import { loadPersisted, savePersisted } from '../lib/storage';
import { setI18nLanguage } from '../i18n';

// index.html EDITMODE のデフォルトに合わせる
export const DEFAULT_TWEAKS: Tweaks = {
  theme: 'light',
  density: 'compact',
  accent: 'green',
  drawerPos: 'bottom',
  lang: 'ja',
  layout: 'standard',
};

const LAYOUTS: readonly Layout[] = ['standard', 'workbench'];

// Tweaks はどこから useTweaks() を呼んでも同一の値を参照する必要があるため、
// コンポーネントローカルな useState ではなくモジュールレベルの共有ストアで管理し、
// useSyncExternalStore で購読する (呼び出し元ごとに state が分断されると、
// TweaksPanel での変更が App 側の props (drawerPos 等) に伝わらない)。
let state: Tweaks | null = null;
const listeners = new Set<() => void>();

function getSnapshot(): Tweaks {
  if (state === null) {
    // layout は docs/issues/closed/0205 以前は使われていない 'tabs-top' が永続化されていた。
    // 既知の値 (standard / workbench) 以外は standard に読み替える (docs/issues/closed/0020 の
    // migrateSessions と同じ、冪等な正規化)。
    const persisted: Partial<Tweaks> & { layout?: unknown } = { ...(loadPersisted().tweaks ?? {}) };
    const layout = LAYOUTS.find((l) => l === persisted.layout) ?? DEFAULT_TWEAKS.layout;
    state = { ...DEFAULT_TWEAKS, ...persisted, layout };
  }
  return state;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function setSharedTweaks(action: Tweaks | ((prev: Tweaks) => Tweaks)): void {
  const next = typeof action === 'function' ? action(getSnapshot()) : action;
  state = next;
  listeners.forEach((listener) => listener());
}

// resetTweaksForTest はテスト間の分離のために共有ストアを未初期化状態へ戻す。テスト専用。
export function resetTweaksForTest(): void {
  state = null;
  listeners.clear();
}

export function useTweaks() {
  const tweaks = useSyncExternalStore(subscribe, getSnapshot);

  useEffect(() => {
    // 永続化 (他フィールドを保持したままマージ)
    const prev = loadPersisted();
    savePersisted({ ...prev, tweaks });

    // ルート要素に data-* 属性を反映
    const root = document.documentElement;
    root.setAttribute('data-theme', tweaks.theme);
    root.setAttribute('data-density', tweaks.density);
    root.setAttribute('data-accent', tweaks.accent);
    root.setAttribute('data-layout', tweaks.layout);

    setI18nLanguage(tweaks.lang);
  }, [tweaks]);

  const update = useCallback((patch: Partial<Tweaks>) => {
    setSharedTweaks((prev) => ({ ...prev, ...patch }));
  }, []);

  return { tweaks, setTweaks: setSharedTweaks, update };
}
