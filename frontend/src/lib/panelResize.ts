// パネル幅のドラッグリサイズ処理の一般化版。sidebarResize.ts (メインサイドバー、左端が
// viewport の x=0) と異なり、コンテンツ領域内で左端に非ゼロのオフセットを持つパネル
// (ログビューア / スキーマツリーの左ツリー) にも対応できるよう、幅の算出基準を
// getLeftEdge() で注入する。
import type { PointerEvent as ReactPointerEvent } from 'react';

export interface PanelResizeOptions {
  min: number;
  max: number;
  cssVar: string;
  // ドラッグ開始時に呼ばれ、幅算出の基準となるパネル左端の clientX を返す。
  getLeftEdge: () => number;
  onWidthChange?: (width: number) => void;
}

// startPanelResize は要素の onPointerDown ハンドラを生成する。
// ドラッグ中は CSS 変数 cssVar を直接更新し、onWidthChange で呼び出し側へ通知する。
export function startPanelResize(opts: PanelResizeOptions) {
  return (e: ReactPointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const left = opts.getLeftEdge();
    const move = (ev: PointerEvent) => {
      const width = Math.min(Math.max(ev.clientX - left, opts.min), opts.max);
      document.documentElement.style.setProperty(opts.cssVar, `${width}px`);
      opts.onWidthChange?.(width);
    };
    const up = () => {
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', up);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
    document.addEventListener('pointermove', move);
    document.addEventListener('pointerup', up);
    document.body.style.cursor = 'ew-resize';
    document.body.style.userSelect = 'none';
  };
}
