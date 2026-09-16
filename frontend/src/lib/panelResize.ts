// パネル幅のドラッグリサイズ処理の一般化版。sidebarResize.ts (メインサイドバー、左端が
// viewport の x=0) と異なり、コンテンツ領域内で左端に非ゼロのオフセットを持つパネル
// (ログビューア / スキーマツリーの左ツリー) にも対応できるよう、幅の算出基準を
// getLeftEdge() で注入する。
// 縦方向 (常駐ターミナルドックの高さ) は startPanelHeightResize が扱う。
import type { PointerEvent as ReactPointerEvent } from 'react';

export interface PanelResizeOptions {
  min: number;
  max: number;
  cssVar: string;
  // ドラッグ開始時に呼ばれ、幅算出の基準となるパネル左端の clientX を返す。
  getLeftEdge: () => number;
  onWidthChange?: (width: number) => void;
}

export interface PanelHeightResizeOptions {
  min: number;
  max: number;
  cssVar: string;
  // ドラッグ開始時に呼ばれ、高さ算出の基準となるパネル下端の clientY を返す。
  getBottomEdge: () => number;
  onHeightChange?: (height: number) => void;
}

// document へのリスナー登録と解除、ドラッグ中の body のカーソルと選択抑止を 1 か所に寄せる。
// 横方向と縦方向で異なるのはカーソルと 1 回の移動あたりの寸法の算出だけ。
function beginDrag(cursor: 'ew-resize' | 'ns-resize', move: (ev: PointerEvent) => void) {
  const up = () => {
    document.removeEventListener('pointermove', move);
    document.removeEventListener('pointerup', up);
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
  };
  document.addEventListener('pointermove', move);
  document.addEventListener('pointerup', up);
  document.body.style.cursor = cursor;
  document.body.style.userSelect = 'none';
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

// startPanelResize は要素の onPointerDown ハンドラを生成する。
// ドラッグ中は CSS 変数 cssVar を直接更新し、onWidthChange で呼び出し側へ通知する。
export function startPanelResize(opts: PanelResizeOptions) {
  return (e: ReactPointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const left = opts.getLeftEdge();
    beginDrag('ew-resize', (ev: PointerEvent) => {
      const width = clamp(ev.clientX - left, opts.min, opts.max);
      document.documentElement.style.setProperty(opts.cssVar, `${width}px`);
      opts.onWidthChange?.(width);
    });
  };
}

// startPanelHeightResize は上端にハンドルを持つパネル (画面下部に固定されたドック) 向けの
// onPointerDown ハンドラを生成する。高さはパネル下端からポインタまでの距離で決まる。
export function startPanelHeightResize(opts: PanelHeightResizeOptions) {
  return (e: ReactPointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const bottom = opts.getBottomEdge();
    beginDrag('ns-resize', (ev: PointerEvent) => {
      const height = clamp(bottom - ev.clientY, opts.min, opts.max);
      document.documentElement.style.setProperty(opts.cssVar, `${height}px`);
      opts.onHeightChange?.(height);
    });
  };
}
