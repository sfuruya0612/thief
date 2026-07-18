// Athena / CloudWatch Logs / BigQuery / Cloud Logging の左パネル (.qe-schema / .lv-tree) の
// 幅を管理するフック。4 ビュー共通の 1 つの幅として CSS 変数 --resource-panel-w に反映し、
// localStorage (storage.ts の resourcePanelWidth) に永続化する。usePersistedSidebarWidth
// (App.tsx) と同じパターン。
import { useCallback, useEffect, useState } from 'react';
import { loadPersisted, savePersisted } from '../lib/storage';

export const RESOURCE_PANEL_MIN_WIDTH = 200;
export const RESOURCE_PANEL_MAX_WIDTH = 480;
export const RESOURCE_PANEL_DEFAULT_WIDTH = 248;
export const RESOURCE_PANEL_CSS_VAR = '--resource-panel-w';

export function useResourcePanelWidth() {
  const [width, setWidthState] = useState<number>(
    () => loadPersisted().resourcePanelWidth ?? RESOURCE_PANEL_DEFAULT_WIDTH,
  );

  useEffect(() => {
    document.documentElement.style.setProperty(RESOURCE_PANEL_CSS_VAR, `${width}px`);
  }, [width]);

  const setWidth = useCallback((w: number) => {
    setWidthState(w);
    const prev = loadPersisted();
    savePersisted({ ...prev, resourcePanelWidth: w });
  }, []);

  return { width, setWidth };
}
