// GCP 用セッションタブの組立レイヤ。プロジェクト一覧の refresh ボタンは
// 旧 GcpSidebar から移設し、ピッカーのヘッダーに置く (一覧の鮮度が問題になる
// のは追加操作のときのため)。
import { useMemo } from 'react';
import { useRefreshGcpProjects } from '../../api/queries';
import type { GcpSessions } from '../../hooks/useGcpProjects';
import { gcpPickerItems, projectEnv } from '../../lib/sessionMeta';
import { Icons } from '../icons/Icons';
import { AddSessionPicker } from './AddSessionPicker';
import { SessionTabs, type SessionTabItem } from './SessionTabs';

export interface GcpSessionTabsProps {
  sessions: GcpSessions;
}

export function GcpSessionTabs({ sessions }: GcpSessionTabsProps) {
  const { projects, openProjects, activeProject, isError } = sessions;
  const refreshProjects = useRefreshGcpProjects();

  const items = useMemo<SessionTabItem[]>(
    () => openProjects.map((id) => ({ id, label: id, env: projectEnv(id) })),
    [openProjects],
  );

  const missingIds = useMemo(() => {
    if (projects.length === 0) return [];
    const known = new Set(projects.map((p) => p.id));
    return openProjects.filter((id) => !known.has(id));
  }, [projects, openProjects]);

  const pickerItems = useMemo(
    () => gcpPickerItems(projects, openProjects),
    [projects, openProjects],
  );

  return (
    <SessionTabs
      items={items}
      activeId={activeProject}
      addLabel="＋ プロジェクトを追加"
      missingIds={missingIds}
      picker={(close, visibleCount) => (
        <AddSessionPicker
          items={pickerItems}
          placeholder="プロジェクトを検索…"
          headerNote="gcloud projects list"
          headerAction={
            <button
              className="btn sm ghost"
              style={{ padding: '1px 4px' }}
              title="Refresh project list from Cloud Resource Manager"
              disabled={refreshProjects.isPending}
              onClick={() => refreshProjects.mutate()}
            >
              <Icons.refresh size={11} />
            </button>
          }
          footerHint="別アカウントのプロジェクトは gcloud auth login で認証後に表示"
          emptyText="一致するプロジェクトがありません"
          loadError={isError}
          onRetry={() => refreshProjects.mutate()}
          narrow
          onSelect={(id) => {
            sessions.openProject(id);
            sessions.swapProjectToVisible(id, visibleCount);
            close();
          }}
          onClose={close}
        />
      )}
      onActivate={sessions.activateProject}
      onClose={sessions.closeProject}
      onReorder={sessions.moveProject}
      onSwapToVisible={sessions.swapProjectToVisible}
    />
  );
}
