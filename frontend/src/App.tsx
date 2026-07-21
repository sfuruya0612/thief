// app.jsx App root の移植: TopBar + AccountView (+ TweaksPanel) を配置する
// AWS 以外 (GCP/Datadog/TiDB) はトップレベルビュー切替で表示する
// profile/region の select はサイドバー (Sidebar.tsx の profile-card) に集約している
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import type { AppView } from './types/common';
import { useHealthCheck } from './api/queries';
import { useProfiles } from './hooks/useProfiles';
import { useActiveGcpProject } from './hooks/useGcpProjects';
import { useTweaks } from './hooks/useTweaks';
import { loadPersisted, savePersisted } from './lib/storage';
import { ConnectionWaiting } from './components/ConnectionWaiting';
import { TopBar } from './components/TopBar';
import { TweaksPanel } from './components/TweaksPanel';
import { AwsSessionTabs } from './components/session/AwsSessionTabs';
import { GcpSessionTabs } from './components/session/GcpSessionTabs';
import { SessionEmptyState } from './components/session/SessionEmptyState';
import { AccountView } from './views/AccountView';
import { GcpView } from './views/GcpView';
import { DatadogView } from './views/nonaws/DatadogView';
import { TiDBView } from './views/nonaws/TiDBView';

const DEFAULT_REGION = 'ap-northeast-1';
const DEFAULT_SIDEBAR_WIDTH = 216;

function usePersistedView() {
  const [view, setViewState] = useState<AppView>(() => loadPersisted().view ?? 'aws');

  useEffect(() => {
    const prev = loadPersisted();
    savePersisted({ ...prev, view });
  }, [view]);

  const setView = useCallback((v: AppView) => setViewState(v), []);
  return { view, setView };
}

// region の永続化状態を管理するローカルフック (storage.ts の region フィールドを使う)
function usePersistedRegion() {
  const [region, setRegionState] = useState<string>(() => loadPersisted().region ?? DEFAULT_REGION);

  useEffect(() => {
    const prev = loadPersisted();
    savePersisted({ ...prev, region });
  }, [region]);

  const setRegion = useCallback((r: string) => setRegionState(r), []);
  return { region, setRegion };
}

// サイドバー幅の永続化状態を管理するローカルフック (storage.ts の sidebarWidth フィールドを使う)
// ドラッグ操作は Sidebar.tsx 側で CSS 変数 --sidebar-w を直接更新するため、ここでは
// 初回マウント時の反映と localStorage への保存のみを担う。
function usePersistedSidebarWidth() {
  const [width, setWidthState] = useState<number>(
    () => loadPersisted().sidebarWidth ?? DEFAULT_SIDEBAR_WIDTH,
  );

  useEffect(() => {
    document.documentElement.style.setProperty('--sidebar-w', `${width}px`);
  }, [width]);

  const setWidth = useCallback((w: number) => {
    setWidthState(w);
    const prev = loadPersisted();
    savePersisted({ ...prev, sidebarWidth: w });
  }, []);
  return { width, setWidth };
}

export function App() {
  const { t } = useTranslation('app');
  const health = useHealthCheck();
  const { tweaks } = useTweaks();
  const aws = useProfiles();
  const { profiles, activeProfile, error } = aws;
  const gcp = useActiveGcpProject();
  const { projects: gcpProjects, activeProject: gcpProject } = gcp;
  const { region, setRegion } = usePersistedRegion();
  const { view, setView } = usePersistedView();
  const { setWidth: setSidebarWidth } = usePersistedSidebarWidth();
  const [tweaksOpen, setTweaksOpen] = useState(false);
  const [activeService, setActiveService] = useState('ec2');
  const [activeGcpService, setActiveGcpService] = useState('cloudrun');
  const queryClient = useQueryClient();

  useEffect(() => {
    if (error) {
      // eslint-disable-next-line no-console
      console.error('failed to load profiles', error);
    }
  }, [error]);

  const handleRefresh = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: [view] });
    // BigQuery のクエリキーは歴史的経緯で 'gcp' ではなく 'bigquery' 始まりのため合わせて更新する
    if (view === 'gcp') {
      void queryClient.invalidateQueries({ queryKey: ['bigquery'] });
    }
  }, [queryClient, view]);

  if (!health.isSuccess) {
    return <ConnectionWaiting />;
  }

  return (
    <div className="app">
      <TopBar
        onToggleTweaks={() => setTweaksOpen((v) => !v)}
        onRefresh={handleRefresh}
        view={view}
        onViewChange={setView}
      />
      {view === 'aws' && <AwsSessionTabs sessions={aws} />}
      {view === 'gcp' && <GcpSessionTabs sessions={gcp} />}
      {view === 'aws' &&
        (activeProfile ? (
          // key= でプロファイル切替時に丸ごと再マウントする。ServicePanel の
          // フィルタ / 選択 / Drawer が前アカウントの状態のまま残るのを防ぐ
          // (エディタ内容は localStorage、リソースは queryKey キャッシュから復元される)。
          <AccountView
            key={activeProfile}
            profile={activeProfile}
            region={region}
            profiles={profiles}
            onRegionChange={setRegion}
            activeService={activeService}
            onServiceChange={setActiveService}
            drawerPos={tweaks.drawerPos}
            onSidebarWidthChange={setSidebarWidth}
          />
        ) : (
          <SessionEmptyState title={t('emptyState.aws.title')} hint={t('emptyState.aws.hint')} />
        ))}
      {view === 'gcp' &&
        (gcpProject ? (
          <GcpView
            key={gcpProject}
            activeProject={gcpProject}
            projects={gcpProjects}
            activeService={activeGcpService}
            onServiceChange={setActiveGcpService}
            drawerPos={tweaks.drawerPos}
            onSidebarWidthChange={setSidebarWidth}
          />
        ) : (
          <SessionEmptyState title={t('emptyState.gcp.title')} hint={t('emptyState.gcp.hint')} />
        ))}
      {view === 'datadog' && <DatadogView />}
      {view === 'tidb' && <TiDBView />}
      {tweaksOpen && <TweaksPanel open onClose={() => setTweaksOpen(false)} />}
    </div>
  );
}
