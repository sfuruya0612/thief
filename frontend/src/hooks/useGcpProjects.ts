// GCP プロジェクトのセッションタブ状態を管理するフック。
// 一覧取得 (useGcpProjects in api/queries.ts) と役割を分けるため、こちらは
// useActiveGcpProject という名前にしてある。
// 旧実装にあった「永続値が一覧に無ければ先頭へ自動フォールバック」は削除した。
// プロジェクト一覧の refresh で消えたプロジェクトのタブが勝手に別プロジェクト
// へ飛ぶのを防ぐため、一覧に無い開きタブもそのまま残す (手動で × で閉じる)。
import { useEffect, useRef } from 'react';
import { useGcpProjects as useGcpProjectsQuery } from '../api/queries';
import type { GcpProject } from '../types/gcp';
import { useSessionTabs } from './useSessionTabs';

export interface GcpSessions {
  projects: GcpProject[];
  isLoading: boolean;
  isError: boolean;
  error: Error | null;
  openProjects: string[];
  activeProject: string;
  activateProject: (id: string) => void;
  openProject: (id: string) => void;
  closeProject: (id: string) => void;
  moveProject: (from: number, to: number) => void;
  swapProjectToVisible: (id: string, visibleCount: number) => void;
}

export function useActiveGcpProject(): GcpSessions {
  const query = useGcpProjectsQuery();
  const tabs = useSessionTabs('gcpSessions');

  // 初回だけの自動オープン (useProfiles と同じワンショット規約)。
  const autoOpened = useRef(false);
  const { open, openSession } = tabs;
  useEffect(() => {
    if (autoOpened.current) return;
    if (!query.data || query.data.length === 0) return;
    autoOpened.current = true;
    if (open.length === 0) {
      openSession(query.data[0].id);
    }
  }, [query.data, open.length, openSession]);

  return {
    projects: query.data ?? [],
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    openProjects: tabs.open,
    activeProject: tabs.active,
    activateProject: tabs.activate,
    openProject: tabs.openSession,
    closeProject: tabs.closeSession,
    moveProject: tabs.move,
    swapProjectToVisible: tabs.swapToVisible,
  };
}
