// セッションタブ状態 (開いているセッション + アクティブ) の永続化フック。
// 遷移ロジックは lib/sessionTabsState.ts の純関数に集約し、ここは
// useState + localStorage 保存の薄いラッパーに徹する。
// このフックは useState ベースのため、同一 scope を複数箇所から呼ぶと state が
// 分断される。必ず App.tsx で 1 回だけ呼び、子へは props で配ること。
import { useCallback, useEffect, useState } from 'react';
import {
  EMPTY_SESSIONS,
  activateSession,
  closeSession,
  moveSession,
  openSession,
  swapSessionToVisible,
  type SessionTabsState,
} from '../lib/sessionTabsState';
import { loadPersisted, savePersisted } from '../lib/storage';

export type SessionScope = 'awsSessions' | 'gcpSessions';

// 旧バージョン互換のミラー先フィールド。アクティブタブを旧形式の単一選択
// フィールドへ常に反映する (ロールバック時も選択が生きる)。
const MIRROR_FIELD: Record<SessionScope, 'activeProfile' | 'gcpProject'> = {
  awsSessions: 'activeProfile',
  gcpSessions: 'gcpProject',
};

export interface SessionTabsApi {
  open: string[];
  active: string;
  activate: (id: string) => void;
  openSession: (id: string) => void;
  closeSession: (id: string) => void;
  move: (from: number, to: number) => void;
  swapToVisible: (id: string, visibleCount: number) => void;
}

export function useSessionTabs(scope: SessionScope): SessionTabsApi {
  const [state, setState] = useState<SessionTabsState>(
    () => loadPersisted()[scope] ?? EMPTY_SESSIONS,
  );

  useEffect(() => {
    // 他の永続化フックとの read-modify-write 交錯を避けるため、保存直前に
    // 必ず最新の PersistedState を読み直してマージする (全 writer が同期実行
    // のため JS 単一スレッドでは安全)。
    const next = { ...loadPersisted(), [scope]: state };
    if (state.active !== '') {
      next[MIRROR_FIELD[scope]] = state.active;
    } else {
      // 全タブ閉のときはミラーを削除する。旧フィールドと active の食い違いを
      // 「旧バージョンが書いた証拠」として扱う reconcile (storage.ts) の前提。
      delete next[MIRROR_FIELD[scope]];
    }
    savePersisted(next);
  }, [scope, state]);

  const activate = useCallback((id: string) => setState((s) => activateSession(s, id)), []);
  const open = useCallback((id: string) => setState((s) => openSession(s, id)), []);
  const close = useCallback((id: string) => setState((s) => closeSession(s, id)), []);
  const move = useCallback(
    (from: number, to: number) => setState((s) => moveSession(s, from, to)),
    [],
  );
  const swapToVisible = useCallback(
    (id: string, visibleCount: number) =>
      setState((s) => swapSessionToVisible(s, id, visibleCount)),
    [],
  );

  return {
    open: state.open,
    active: state.active,
    activate,
    openSession: open,
    closeSession: close,
    move,
    swapToVisible,
  };
}
