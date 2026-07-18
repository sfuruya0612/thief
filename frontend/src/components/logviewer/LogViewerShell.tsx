// ログビューアの共通レイアウト。上部ツールバー (タイトル + 操作) と、左ツリー + 右 (フィルタ /
// ヒストグラム + ログ一覧) の分割ボディを組む。各スロットの中身は呼び出し側 (CloudWatch Logs /
// Cloud Logging) が差し込む。
import { useRef, type ReactNode } from 'react';
import {
  RESOURCE_PANEL_CSS_VAR,
  RESOURCE_PANEL_MAX_WIDTH,
  RESOURCE_PANEL_MIN_WIDTH,
  useResourcePanelWidth,
} from '../../hooks/useResourcePanelWidth';
import { startPanelResize } from '../../lib/panelResize';

export interface LogViewerShellProps {
  title: string;
  subtitle?: string;
  // ツールバー右の操作群 (ライブテールトグル・期間指定・エクスポート等)。
  toolbarActions?: ReactNode;
  tree: ReactNode;
  // フィルタ式の入力行 (エディタ + スニペット + 実行ボタン)。
  filterBar: ReactNode;
  // ヒストグラム領域 (キャプション + バー + 時間軸)。null なら非表示。
  histogram?: ReactNode;
  // ログ一覧 (見出し・本体・フッターを内包)。
  logList: ReactNode;
  // ボディ上部に出すバナー (エラー / SSO 期限切れ等)。
  banner?: ReactNode;
}

export function LogViewerShell({
  title,
  subtitle,
  toolbarActions,
  tree,
  filterBar,
  histogram,
  logList,
  banner,
}: LogViewerShellProps) {
  const treeRef = useRef<HTMLDivElement>(null);
  const { setWidth } = useResourcePanelWidth();

  return (
    <div className="main lv-root">
      <div className="lv-topbar">
        <div className="lv-topbar-title">
          <h1>{title}</h1>
          {subtitle && <span className="lv-subtitle">{subtitle}</span>}
        </div>
        {toolbarActions && <div className="lv-topbar-actions">{toolbarActions}</div>}
      </div>

      {banner}

      <div className="lv-body">
        <div className="lv-tree" ref={treeRef}>
          {tree}
          <div
            className="panel-resizer"
            onPointerDown={startPanelResize({
              min: RESOURCE_PANEL_MIN_WIDTH,
              max: RESOURCE_PANEL_MAX_WIDTH,
              cssVar: RESOURCE_PANEL_CSS_VAR,
              getLeftEdge: () => treeRef.current?.getBoundingClientRect().left ?? 0,
              onWidthChange: setWidth,
            })}
            title="Drag to resize"
          />
        </div>
        <div className="lv-right">
          <div className="lv-filter-card">
            <div className="lv-filter-bar">{filterBar}</div>
            {histogram && <div className="lv-histogram-wrap">{histogram}</div>}
          </div>
          <div className="lv-list-card">{logList}</div>
        </div>
      </div>
    </div>
  );
}
