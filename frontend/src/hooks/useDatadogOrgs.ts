// Datadog の組織 (親組織と Sub Organization) のセッションタブ状態を管理するフック。
// 一覧取得 (useDatadogOrgs in api/queries.ts) と役割を分けるため、こちらは
// useActiveDatadogOrg という名前にしてある (useActiveGcpProject と対称)。
// 一覧に無い開きタブもそのまま残す (勝手に別の組織へ飛ばさない)。
import { useEffect, useRef } from 'react';
import { useDatadogOrgs as useDatadogOrgsQuery } from '../api/queries';
import type { DatadogOrgRow } from '../types/nonaws';
import { useSessionTabs } from './useSessionTabs';

export interface DatadogOrgSessions {
  orgs: DatadogOrgRow[];
  isLoading: boolean;
  isError: boolean;
  error: Error | null;
  openOrgs: string[];
  activeOrg: string;
  activateOrg: (id: string) => void;
  openOrg: (id: string) => void;
  closeOrg: (id: string) => void;
  moveOrg: (from: number, to: number) => void;
  swapOrgToVisible: (id: string, visibleCount: number) => void;
}

export function useActiveDatadogOrg(): DatadogOrgSessions {
  const query = useDatadogOrgsQuery();
  const tabs = useSessionTabs('datadogOrgSessions');

  // 初回だけの自動オープン (useProfiles / useActiveGcpProject と同じワンショット規約)。
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
    orgs: query.data ?? [],
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    openOrgs: tabs.open,
    activeOrg: tabs.active,
    activateOrg: tabs.activate,
    openOrg: tabs.openSession,
    closeOrg: tabs.closeSession,
    moveOrg: tabs.move,
    swapOrgToVisible: tabs.swapToVisible,
  };
}
