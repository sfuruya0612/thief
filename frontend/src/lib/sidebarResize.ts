// サイドバー幅のドラッグリサイズ処理。Sidebar (AWS) と GcpSidebar で共用する。
import type { PointerEvent as ReactPointerEvent } from 'react';

const SIDEBAR_MIN_WIDTH = 160;
const SIDEBAR_MAX_WIDTH = 480;

// startSidebarResize は .sidebar-resizer の onPointerDown ハンドラを生成する。
// ドラッグ中は CSS 変数 --sidebar-w を直接更新し、onWidthChange で親へ通知する。
export function startSidebarResize(onWidthChange?: (width: number) => void) {
  return (e: ReactPointerEvent<HTMLDivElement>) => {
    e.preventDefault();
    const move = (ev: PointerEvent) => {
      const width = Math.min(Math.max(ev.clientX, SIDEBAR_MIN_WIDTH), SIDEBAR_MAX_WIDTH);
      document.documentElement.style.setProperty('--sidebar-w', `${width}px`);
      onWidthChange?.(width);
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
