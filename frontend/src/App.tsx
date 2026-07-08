// app.jsx App root の移植: TopBar + AccountView (+ TweaksPanel) を配置する
// AWS 以外 (BigQuery/Datadog/TiDB) はトップレベルビュー切替で表示する (Phase 5 で新規追加)
// profile/region の select はサイドバー (Sidebar.tsx の profile-card) に集約している
import { useCallback, useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { AppView } from './types/common';
import { useProfiles } from './hooks/useProfiles';
import { useTweaks } from './hooks/useTweaks';
import { loadPersisted, savePersisted } from './lib/storage';
import { TopBar } from './components/TopBar';
import { TweaksPanel } from './components/TweaksPanel';
import { StatusBar } from './components/StatusBar';
import { AccountView } from './views/AccountView';
import { BigQueryView } from './views/nonaws/BigQueryView';
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
  const { tweaks, update } = useTweaks();
  const { profiles, activeProfile, setActiveProfile, error } = useProfiles();
  const { region, setRegion } = usePersistedRegion();
  const { view, setView } = usePersistedView();
  const { setWidth: setSidebarWidth } = usePersistedSidebarWidth();
  const [tweaksOpen, setTweaksOpen] = useState(false);
  const [activeService, setActiveService] = useState('ec2');
  const queryClient = useQueryClient();

  // フッター (StatusBar) に表示するサービス名: AWS ビューは選択中サービス、非 AWS はビュー名そのもの
  const footerService = view === 'aws' ? activeService : view;

  useEffect(() => {
    if (error) {
      // eslint-disable-next-line no-console
      console.error('failed to load profiles', error);
    }
  }, [error]);

  const handleRefresh = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: [view] });
  }, [queryClient, view]);

  const handleToggleTheme = useCallback(() => {
    update({ theme: tweaks.theme === 'dark' ? 'light' : 'dark' });
  }, [tweaks.theme, update]);

  return (
    <div className="app">
      <TopBar
        theme={tweaks.theme}
        onToggleTheme={handleToggleTheme}
        onToggleTweaks={() => setTweaksOpen((v) => !v)}
        onRefresh={handleRefresh}
        view={view}
        onViewChange={setView}
      />
      {view === 'aws' && activeProfile && (
        <AccountView
          profile={activeProfile}
          region={region}
          profiles={profiles}
          onProfileChange={setActiveProfile}
          onRegionChange={setRegion}
          activeService={activeService}
          onServiceChange={setActiveService}
          showMiniCharts={tweaks.showMiniCharts}
          drawerPos={tweaks.drawerPos}
          onSidebarWidthChange={setSidebarWidth}
        />
      )}
      {view === 'bigquery' && <BigQueryView />}
      {view === 'datadog' && <DatadogView />}
      {view === 'tidb' && <TiDBView />}
      <StatusBar service={footerService} />
      {tweaksOpen && <TweaksPanel open onClose={() => setTweaksOpen(false)} />}
    </div>
  );
}
