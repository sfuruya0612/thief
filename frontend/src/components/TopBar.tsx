// app.jsx TopBar の移植
// + AWS/BigQuery/Datadog/TiDB のトップレベルビュー切替 (Phase 5 で新規追加)
// profile/region セレクタはサイドバーの profile-card へ移設済み (Sidebar.tsx を参照)
import type { AppView, Theme } from '../types/common';
import { Icons } from './icons/Icons';

const VIEWS: Array<[AppView, string]> = [
  ['aws', 'AWS'],
  ['bigquery', 'BigQuery'],
  ['datadog', 'Datadog'],
  ['tidb', 'TiDB'],
];

export interface TopBarProps {
  theme: Theme;
  onToggleTheme: () => void;
  onToggleTweaks: () => void;
  onRefresh: () => void;
  view: AppView;
  onViewChange: (view: AppView) => void;
}

export function TopBar({
  theme,
  onToggleTheme,
  onToggleTweaks,
  onRefresh,
  view,
  onViewChange,
}: TopBarProps) {
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
      <div className="spacer" />
      <button className="iconbtn" title="Refresh" onClick={onRefresh}>
        <Icons.refresh />
      </button>
      <button
        className="iconbtn"
        title={`Switch to ${theme === 'dark' ? 'light' : 'dark'}`}
        onClick={onToggleTheme}
      >
        {theme === 'dark' ? <Icons.sun /> : <Icons.moon />}
      </button>
      <button className="iconbtn" title="Tweaks" onClick={onToggleTweaks}>
        <Icons.settings />
      </button>
    </div>
  );
}
