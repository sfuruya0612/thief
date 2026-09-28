// サイドバーを rail に畳む / 展開するボタン (issue 0211)。Sidebar (AWS) と GcpSidebar で共用する。
// workbench レイアウトでだけ描画され、⌘B / Ctrl+B と同じ操作をする。
import { useTranslation } from 'react-i18next';
import { Icons } from './icons/Icons';

export interface SidebarToggleProps {
  collapsed: boolean;
  onToggle: () => void;
}

export function SidebarToggle({ collapsed, onToggle }: SidebarToggleProps) {
  const { t } = useTranslation('sidebar');
  const label = collapsed ? t('sidebar.expand') : t('sidebar.collapse');
  return (
    <button
      className="sidebar-toggle"
      title={`${label} (⌘B / Ctrl+B)`}
      aria-label={label}
      aria-expanded={!collapsed}
      onClick={onToggle}
    >
      <Icons.chevron size={12} style={{ transform: collapsed ? 'none' : 'rotate(180deg)' }} />
      {!collapsed && <span className="nav-label">{label}</span>}
    </button>
  );
}
