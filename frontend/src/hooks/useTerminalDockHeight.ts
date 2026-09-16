// 常駐ターミナルドック (components/Terminal/TerminalDock.tsx) の本文 (div.terminal-dock-body) の
// 高さを管理するフック。保持する値は本文の高さ (px) で、タブバーの高さ (TAB_BAR_HEIGHT) は
// レイアウトの定数として別に扱う。localStorage (storage.ts の terminalDockHeight) に永続化する。
// useResourcePanelWidth と同じパターン。
//
// CSS 変数への反映は行わない。ドックの高さを表す --terminal-dock-h は「ルート要素の高さ」
// (折りたたみ時とセッション 0 件のときを含む) を意味し、TerminalDock がその全ての場合を
// まとめて設定するため。
import { useCallback, useState } from 'react';
import { loadPersisted, savePersisted } from '../lib/storage';

// タブバーの高さ。ドックのルート要素の高さ (折りたたみ時) と一致する。
export const TAB_BAR_HEIGHT = 32;
// 本文の高さの既定値。
export const TERMINAL_DOCK_DEFAULT_BODY_HEIGHT = 320;
// 本文の高さの下限。xterm の fontSize 13 に対し、.terminal-container の padding 8px と
// border 1px を除いた 142px へ 8 行から 9 行程度が入る (セルの高さはフォントに依存するため概算)。
// コマンド 1 つとその出力数行を見る最小の面積として決めた値。
export const TERMINAL_DOCK_MIN_BODY_HEIGHT = 160;
// ルート要素の高さの上限がウィンドウ高さに占める割合。Drawer のクランプ (issue 0003) と
// 同じ係数にし、ドックと Drawer で画面に残すメインコンテンツの割合を揃える。
export const TERMINAL_DOCK_MAX_HEIGHT_RATIO = 0.85;

// 本文の高さの下限と上限。ドラッグ中のクランプ (TerminalDock が startPanelHeightResize へ
// 渡す min / max) と描画時の再クランプ (clampTerminalDockBodyHeight) の両方がこの 1 か所から
// 範囲を得るため、下限・係数・TAB_BAR_HEIGHT のどれを変えても両者は食い違わない。
export function terminalDockBodyHeightRange(innerHeight: number): { min: number; max: number } {
  const min = TERMINAL_DOCK_MIN_BODY_HEIGHT;
  // 上限が下限を下回るほど小さいウィンドウでは max も下限に合わせる。
  const max = Math.max(
    min,
    Math.round(innerHeight * TERMINAL_DOCK_MAX_HEIGHT_RATIO) - TAB_BAR_HEIGHT,
  );
  return { min, max };
}

// 永続化された本文の高さを、現在のウィンドウ高さに対する範囲へ丸める。
export function clampTerminalDockBodyHeight(bodyHeight: number, innerHeight: number): number {
  const { min, max } = terminalDockBodyHeightRange(innerHeight);
  return Math.min(Math.max(bodyHeight, min), max);
}

export function useTerminalDockHeight() {
  const [bodyHeight, setBodyHeightState] = useState<number>(() => {
    const stored = loadPersisted().terminalDockHeight;
    // 未設定、手で書き換えた文字列、NaN のいずれでも既定値へ倒す。
    // 値の範囲は描画時に clampTerminalDockBodyHeight が丸める。
    return typeof stored === 'number' && Number.isFinite(stored)
      ? stored
      : TERMINAL_DOCK_DEFAULT_BODY_HEIGHT;
  });

  const setBodyHeight = useCallback((h: number) => {
    setBodyHeightState(h);
    const prev = loadPersisted();
    savePersisted({ ...prev, terminalDockHeight: h });
  }, []);

  return { bodyHeight, setBodyHeight };
}
