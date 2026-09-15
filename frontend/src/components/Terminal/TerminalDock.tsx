// 画面下部に常駐するターミナルドック。
// App 直下 (.app の末尾の子) に置き、view / プロファイル / サービス / リソース選択 /
// Drawer の開閉のどれにも依存しない位置で Terminal をマウントし続ける。開いている
// 全セッションの Terminal を同時にマウントし、非アクティブなものはラッパーに hidden を
// 付けて隠すだけでアンマウントしない (アンマウントは ws.close() を経由して backend の
// SSM セッションを終了させるため)。
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Icons } from '../icons/Icons';
import {
  activateTerminalSession,
  closeTerminalSession,
  setTerminalDockCollapsed,
  useTerminalSessions,
} from '../../hooks/useTerminalSessions';
import { Terminal, type ConnectionStatus } from './Terminal';
import { pruneStatuses } from './terminalDockStatuses';

// タブバーの高さ。ドックのルート要素の高さ (折りたたみ時) と一致する。
const TAB_BAR_HEIGHT = 32;
// 本文 (div.terminal-dock-body) の高さ。app.css の .terminal-panel の min-height と
// 同じ値にする (本文がこれより低いとパネルが本文からあふれる)。
const BODY_HEIGHT = 320;

// Drawer (position: fixed) がドックを覆わないよう、ドックの高さを CSS 変数として
// document.documentElement に反映する (app.css の .drawer / .drawer.pos-bottom の bottom が参照する)。
const DOCK_HEIGHT_VAR = '--terminal-dock-h';

export function TerminalDock() {
  const { t } = useTranslation('drawerStorage');
  const { tabs, sessions, collapsed } = useTerminalSessions();
  const [statuses, setStatuses] = useState<Record<string, ConnectionStatus>>({});

  const hasSessions = tabs.open.length > 0;
  const rootHeight = collapsed ? TAB_BAR_HEIGHT : TAB_BAR_HEIGHT + BODY_HEIGHT;

  useEffect(() => {
    const root = document.documentElement;
    root.style.setProperty(DOCK_HEIGHT_VAR, hasSessions ? `${rootHeight}px` : '0px');
    return () => {
      root.style.setProperty(DOCK_HEIGHT_VAR, '0px');
    };
  }, [hasSessions, rootHeight]);

  const handleStatusChange = useCallback((id: string, status: ConnectionStatus) => {
    setStatuses((prev) => (prev[id] === status ? prev : { ...prev, [id]: status }));
  }, []);

  // セッションが閉じられて tabs.open から消えたら、対応する接続状態も破棄する
  // (堅牢性 > 性能: 開閉を繰り返しても statuses が際限なく蓄積しないようにする)。
  useEffect(() => {
    setStatuses((prev) => pruneStatuses(prev, tabs.open));
  }, [tabs.open]);

  if (!hasSessions) return null;

  const statusLabel = (status: ConnectionStatus | undefined): string => {
    switch (status) {
      case 'connected':
        return t('terminal.statusConnected');
      case 'closed':
        return t('terminal.statusClosed');
      case 'error':
        return t('terminal.statusError');
      default:
        return t('terminal.statusConnecting');
    }
  };

  return (
    <div className="terminal-dock" style={{ height: rootHeight }}>
      <div className="terminal-dock-tabs" style={{ height: TAB_BAR_HEIGHT }} role="tablist">
        {tabs.open.map((id) => {
          const session = sessions[id];
          if (!session) return null;
          const status = statuses[id];
          return (
            <div
              key={id}
              className={`terminal-dock-tab ${id === tabs.active ? 'active' : ''}`}
              role="tab"
              aria-selected={id === tabs.active}
              title={`${session.label} (${session.profile} / ${session.region})`}
              onClick={() => activateTerminalSession(id)}
            >
              <span className={`terminal-dock-tab-status status-${status ?? 'connecting'}`} />
              <span className="terminal-dock-tab-kind">
                {session.kind === 'ec2' ? 'EC2' : 'ECS'}
              </span>
              <span className="terminal-dock-tab-name">{session.label}</span>
              <span className="terminal-dock-tab-state">{statusLabel(status)}</span>
              <button
                className="terminal-dock-tab-close"
                title={t('terminal.closeSession')}
                aria-label={t('terminal.closeSession')}
                onClick={(e) => {
                  e.stopPropagation();
                  closeTerminalSession(id);
                }}
              >
                <Icons.x size={10} />
              </button>
            </div>
          );
        })}
        <button
          className="terminal-dock-toggle"
          title={collapsed ? t('terminal.expandDock') : t('terminal.collapseDock')}
          aria-label={collapsed ? t('terminal.expandDock') : t('terminal.collapseDock')}
          aria-expanded={!collapsed}
          onClick={() => setTerminalDockCollapsed(!collapsed)}
        >
          <Icons.chevron size={12} />
        </button>
      </div>
      <div className="terminal-dock-body" hidden={collapsed}>
        {tabs.open.map((id) => {
          const session = sessions[id];
          if (!session) return null;
          const isActive = id === tabs.active;
          return (
            <div key={id} className="terminal-dock-pane" hidden={!isActive}>
              <Terminal
                wsUrl={session.wsUrl}
                active={isActive && !collapsed}
                onStatusChange={(status) => handleStatusChange(id, status)}
              />
            </div>
          );
        })}
      </div>
    </div>
  );
}
