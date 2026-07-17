// AWS プロファイルのセッションタブ状態を管理するフック。
// 一覧取得 (api/queries.ts の useProfiles) と useSessionTabs('awsSessions') を
// 合成する。useState ベースのため App.tsx で 1 回だけ呼び、子へ props で配る。
import { useEffect, useRef } from 'react';
import { useProfiles as useProfilesQuery } from '../api/queries';
import type { Profile } from '../types/common';
import { useSessionTabs } from './useSessionTabs';

export interface AwsSessions {
  profiles: Profile[];
  isLoading: boolean;
  isError: boolean;
  error: Error | null;
  refetchProfiles: () => void;
  openProfiles: string[];
  activeProfile: string;
  activateProfile: (name: string) => void;
  openProfile: (name: string) => void;
  closeProfile: (name: string) => void;
  moveProfile: (from: number, to: number) => void;
  swapProfileToVisible: (name: string, visibleCount: number) => void;
}

export function useProfiles(): AwsSessions {
  const query = useProfilesQuery();
  const tabs = useSessionTabs('awsSessions');

  // 初回だけの自動オープン: 一覧が最初にロードできた時点で開いているタブが
  // 無ければ先頭を開く。shot は「一覧が非空で来たとき」のみ消費し、error /
  // 空一覧では消費しない (backend 復帰後の refetch で改めて発火できる)。
  // 全タブを手動で閉じた後に自動で開き直さないための useRef ガード。
  // close-all 状態はリロードを跨いで保存しない (リロード後は再び先頭が開く)
  // 意図的な仕様。
  const autoOpened = useRef(false);
  const { open, openSession } = tabs;
  useEffect(() => {
    if (autoOpened.current) return;
    if (!query.data || query.data.length === 0) return;
    autoOpened.current = true;
    if (open.length === 0) {
      openSession(query.data[0].name);
    }
  }, [query.data, open.length, openSession]);

  return {
    profiles: query.data ?? [],
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    refetchProfiles: () => void query.refetch(),
    openProfiles: tabs.open,
    activeProfile: tabs.active,
    activateProfile: tabs.activate,
    openProfile: tabs.openSession,
    closeProfile: tabs.closeSession,
    moveProfile: tabs.move,
    swapProfileToVisible: tabs.swapToVisible,
  };
}
