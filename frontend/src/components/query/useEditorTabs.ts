// エディタタブ状態の管理と localStorage 永続化。
// スコープ (プロジェクト / プロファイル) の切替は呼び出し側で key を変えて再マウントする前提。
import { useCallback, useEffect, useState } from 'react';
import type { QueryTab } from '../../types/query';
import {
  type EditorTabsState,
  loadEditorTabs,
  newLocalId,
  type QueryEditorService,
  saveEditorTabs,
  untitledName,
} from '../../lib/queryEditorStorage';

export interface EditorTabsApi {
  tabs: QueryTab[];
  activeTab: QueryTab;
  activeTabId: string;
  setActive: (id: string) => void;
  updateSql: (id: string, sql: string) => void;
  renameTab: (id: string, name: string) => void;
  addTab: (sql?: string, name?: string) => string;
  closeTab: (id: string) => void;
}

export function useEditorTabs(
  service: QueryEditorService,
  scope: string,
  defaultSql: string,
): EditorTabsApi {
  const [state, setState] = useState<EditorTabsState>(() =>
    loadEditorTabs(service, scope, defaultSql),
  );

  useEffect(() => {
    saveEditorTabs(service, scope, state);
  }, [service, scope, state]);

  const setActive = useCallback((id: string) => {
    setState((s) => (s.tabs.some((t) => t.id === id) ? { ...s, activeTabId: id } : s));
  }, []);

  const updateSql = useCallback((id: string, sql: string) => {
    setState((s) => ({
      ...s,
      tabs: s.tabs.map((t) => (t.id === id ? { ...t, sql } : t)),
    }));
  }, []);

  const renameTab = useCallback((id: string, name: string) => {
    const trimmed = name.trim();
    if (!trimmed) return;
    setState((s) => ({
      ...s,
      tabs: s.tabs.map((t) => (t.id === id ? { ...t, name: trimmed } : t)),
    }));
  }, []);

  const addTab = useCallback((sql = '', name?: string): string => {
    const id = newLocalId('tab');
    setState((s) => ({
      tabs: [...s.tabs, { id, name: name ?? untitledName(s.tabs), sql }],
      activeTabId: id,
    }));
    return id;
  }, []);

  const closeTab = useCallback((id: string) => {
    setState((s) => {
      const idx = s.tabs.findIndex((t) => t.id === id);
      const tabs = s.tabs.filter((t) => t.id !== id);
      if (tabs.length === 0) {
        const tab: QueryTab = { id: newLocalId('tab'), name: 'untitled 1', sql: '' };
        return { tabs: [tab], activeTabId: tab.id };
      }
      const activeTabId = s.activeTabId === id ? tabs[Math.max(0, idx - 1)].id : s.activeTabId;
      return { tabs, activeTabId };
    });
  }, []);

  const activeTab = state.tabs.find((t) => t.id === state.activeTabId) ?? state.tabs[0];

  return {
    tabs: state.tabs,
    activeTab,
    activeTabId: activeTab.id,
    setActive,
    updateSql,
    renameTab,
    addTab,
    closeTab,
  };
}
