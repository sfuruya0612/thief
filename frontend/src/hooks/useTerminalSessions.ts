// 常駐ターミナルドック (components/Terminal/TerminalDock.tsx) が表示する
// EC2 Session Manager / ECS Exec のセッション状態を持つ共有ストア。
//
// セッションを開く操作は Drawer 側 (Terminal タブの Connect ボタン、ECS Tasks タブの
// Exec ボタン) にあり、ターミナル本体は App 直下のドックにマウントされる。両者は
// React のツリー上で親子関係に無いため、useTweaks.ts と同じモジュールレベルの共有ストアを
// useSyncExternalStore で購読する形にし、props のバケツリレーを避ける。
//
// 状態は localStorage に永続化しない。ブラウザの WebSocket が閉じた時点で backend が
// SSM セッションを終了させるため、リロード後の復元は「同じ対象へ新しいセッションを開き直す」
// 操作になり、利用者の操作なしに AWS API を呼ぶことになるため。
import { useSyncExternalStore } from 'react';
import {
  EMPTY_SESSIONS,
  activateSession,
  closeSession,
  openSession,
  type SessionTabsState,
} from '../lib/sessionTabsState';

export interface TerminalSession {
  // ストア内の連番から採番した識別子 (タブの id を兼ねる)
  id: string;
  kind: 'ec2' | 'ecs';
  profile: string;
  region: string;
  // タブに表示する名前。EC2 はインスタンス名 (無ければ id)、
  // ECS は "cluster / task id の末尾 / コンテナ名"
  label: string;
  // 接続先の WebSocket URL (api/terminal.ts の ec2SessionUrl/ecsExecUrl で組み立てる)
  wsUrl: string;
}

export interface TerminalSessionsState {
  tabs: SessionTabsState;
  sessions: Record<string, TerminalSession>;
  // ドックの本文 (ターミナル) を折りたたんでいるか。折りたたんでも Terminal は
  // アンマウントしないため、接続は維持される。
  collapsed: boolean;
}

const INITIAL_STATE: TerminalSessionsState = {
  tabs: EMPTY_SESSIONS,
  sessions: {},
  collapsed: false,
};

let state: TerminalSessionsState = INITIAL_STATE;
let sequence = 0;
const listeners = new Set<() => void>();

function getSnapshot(): TerminalSessionsState {
  return state;
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function setState(next: TerminalSessionsState): void {
  if (next === state) return;
  state = next;
  listeners.forEach((listener) => listener());
}

// セッションを追加してアクティブにし、ドックが折りたたまれていれば展開する。
// 同じ対象への Connect を繰り返すと、そのたびに別 id の新しいセッションが増える
// (1 つのインスタンスで複数シェルを開く使い方があるため UI 側で制限しない)。
export function openTerminalSession(input: Omit<TerminalSession, 'id'>): string {
  sequence += 1;
  const id = `terminal-${sequence}`;
  setState({
    tabs: openSession(state.tabs, id),
    sessions: { ...state.sessions, [id]: { ...input, id } },
    collapsed: false,
  });
  return id;
}

// セッションを削除する。次のアクティブは sessionTabsState.ts の closeSession に従う。
// Terminal のアンマウントに伴う ws.close() が backend の TerminateSSMSession を呼ぶ。
export function closeTerminalSession(id: string): void {
  if (!(id in state.sessions)) return;
  const sessions = { ...state.sessions };
  delete sessions[id];
  setState({ ...state, tabs: closeSession(state.tabs, id), sessions });
}

export function activateTerminalSession(id: string): void {
  const tabs = activateSession(state.tabs, id);
  if (tabs === state.tabs) return;
  setState({ ...state, tabs });
}

export function setTerminalDockCollapsed(collapsed: boolean): void {
  if (state.collapsed === collapsed) return;
  setState({ ...state, collapsed });
}

// resetTerminalSessionsForTest はテスト間の分離のために共有ストアを初期状態へ戻す。テスト専用。
export function resetTerminalSessionsForTest(): void {
  state = INITIAL_STATE;
  sequence = 0;
  listeners.clear();
}

export function useTerminalSessions(): TerminalSessionsState {
  return useSyncExternalStore(subscribe, getSnapshot);
}
