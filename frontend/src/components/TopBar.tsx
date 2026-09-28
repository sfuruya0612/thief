// app.jsx TopBar の移植
// + AWS/GCP/Datadog/TiDB のトップレベルビュー切替
// profile/region セレクタはサイドバーの profile-card へ移設済み (Sidebar.tsx を参照)
import { useTranslation } from 'react-i18next';
import type { ReactNode } from 'react';
import type { AppView } from '../types/common';
import { Icons } from './icons/Icons';

const VIEWS: Array<[AppView, string]> = [
  ['aws', 'AWS'],
  ['gcp', 'Google Cloud'],
  ['datadog', 'Datadog'],
  ['tidb', 'TiDB'],
];

export interface TopBarProps {
  onToggleTweaks: () => void;
  onRefresh: () => void;
  // Refresh 実行中 (backend キャッシュ破棄 + query 無効化) はボタンを無効化する
  refreshing: boolean;
  view: AppView;
  onViewChange: (view: AppView) => void;
  // 分割表示の開始 / 終了。渡されたときだけ分割ボタンを出す (セッションがある
  // AWS / Google Cloud のビューだけ。Datadog / TiDB とセッション未選択では出さない)。
  split?: { active: boolean; onToggle: () => void };
  // workbench レイアウトではセッションタブを TopBar の中 (プロバイダ切替の右) に置く
  // (issue 0211)。standard では App が TopBar の下に別の行として描画するため渡さない。
  sessionTabs?: ReactNode;
}

export function TopBar({
  onToggleTweaks,
  onRefresh,
  refreshing,
  view,
  onViewChange,
  split,
  sessionTabs,
}: TopBarProps) {
  const { t } = useTranslation('topbar');
  return (
    <div className="topbar">
      <div className="brand">
        <img className="logo" src="/assets/thief-compass.png" alt="" />
        <img className="wordmark" src="/assets/thief-wordmark.png" alt="thief" />
      </div>
      <span className="divider" />
      <div className="view-switch">
        {VIEWS.map(([key, label]) => (
          <button
            key={key}
            className={view === key ? 'active' : ''}
            onClick={() => onViewChange(key)}
          >
            {label}
          </button>
        ))}
      </div>
      {sessionTabs && <div className="topbar-sessions">{sessionTabs}</div>}
      <div className="spacer" />
      {split && (
        <button
          className={`iconbtn${split.active ? ' active' : ''}`}
          title={t('split')}
          aria-pressed={split.active}
          onClick={split.onToggle}
        >
          <Icons.split />
        </button>
      )}
      <button className="iconbtn" title="Refresh" onClick={onRefresh} disabled={refreshing}>
        <Icons.refresh />
      </button>
      <button className="iconbtn" title="Tweaks" onClick={onToggleTweaks}>
        <Icons.settings />
      </button>
    </div>
  );
}
