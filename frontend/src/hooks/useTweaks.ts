import { useCallback, useEffect, useState } from 'react';
import type { Tweaks } from '../types/common';
import { loadPersisted, savePersisted } from '../lib/storage';

// index.html EDITMODE のデフォルトに合わせる
export const DEFAULT_TWEAKS: Tweaks = {
  theme: 'light',
  density: 'compact',
  accent: 'green',
  layout: 'tabs-top',
  drawerPos: 'bottom',
  showMiniCharts: true,
};

export function useTweaks() {
  const [tweaks, setTweaks] = useState<Tweaks>(() => {
    const persisted = loadPersisted().tweaks;
    return { ...DEFAULT_TWEAKS, ...(persisted ?? {}) };
  });

  useEffect(() => {
    // 永続化 (他フィールドを保持したままマージ)
    const prev = loadPersisted();
    savePersisted({ ...prev, tweaks });

    // ルート要素に data-* 属性を反映
    const root = document.documentElement;
    root.setAttribute('data-theme', tweaks.theme);
    root.setAttribute('data-density', tweaks.density);
    root.setAttribute('data-accent', tweaks.accent);
  }, [tweaks]);

  const update = useCallback((patch: Partial<Tweaks>) => {
    setTweaks((prev) => ({ ...prev, ...patch }));
  }, []);

  return { tweaks, setTweaks, update };
}
