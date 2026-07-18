// サイドバー幅のドラッグリサイズ処理。Sidebar (AWS) と GcpSidebar で共用する。
import type { PointerEvent as ReactPointerEvent } from 'react';
import { startPanelResize } from './panelResize';

const SIDEBAR_MIN_WIDTH = 160;
const SIDEBAR_MAX_WIDTH = 480;

// startSidebarResize は .sidebar-resizer の onPointerDown ハンドラを生成する。
// ドラッグ中は CSS 変数 --sidebar-w を直接更新し、onWidthChange で親へ通知する。
// メインサイドバーは viewport の左端 (x=0) に固定されているため、左端オフセットは常に 0。
export function startSidebarResize(
  onWidthChange?: (width: number) => void,
): (e: ReactPointerEvent<HTMLDivElement>) => void {
  return startPanelResize({
    min: SIDEBAR_MIN_WIDTH,
    max: SIDEBAR_MAX_WIDTH,
    cssVar: '--sidebar-w',
    getLeftEdge: () => 0,
    onWidthChange,
  });
}
