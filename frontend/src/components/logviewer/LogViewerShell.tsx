// ログビューアの共通レイアウト。上部ツールバー (タイトル + 操作) と、左ツリー + 右 (フィルタ /
// ヒストグラム + ログ一覧) の分割ボディを組む。各スロットの中身は呼び出し側 (CloudWatch Logs /
// Cloud Logging) が差し込む。
import { type ReactNode } from 'react';

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
        <div className="lv-tree">{tree}</div>
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
