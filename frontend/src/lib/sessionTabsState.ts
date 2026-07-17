// セッションタブ (開いている AWS プロファイル / GCP プロジェクト) の状態と
// 遷移ロジック。フック (hooks/useSessionTabs.ts) から分離した純関数群で、
// localStorage 破損や範囲外 index への防御もここに集約する。

export interface SessionTabsState {
  // 開いているセッション ID (プロファイル名 / プロジェクト ID)。並び = タブ表示順
  open: string[];
  // アクティブなセッション ID ('' = 未選択)
  active: string;
}

export const EMPTY_SESSIONS: SessionTabsState = { open: [], active: '' };

// id をアクティブにする。open に無い id は no-op。
export function activateSession(s: SessionTabsState, id: string): SessionTabsState {
  if (!s.open.includes(id) || s.active === id) return s;
  return { ...s, active: id };
}

// id を開く。既に open ならアクティブ化のみ (重複追加しない)。
export function openSession(s: SessionTabsState, id: string): SessionTabsState {
  if (id === '') return s;
  if (s.open.includes(id)) return activateSession(s, id);
  return { open: [...s.open, id], active: id };
}

// id を閉じる。アクティブなタブを閉じた場合は左隣 (無ければ先頭) をアクティブに
// する。最後の 1 個を閉じると { open: [], active: '' } になる (空状態 UI 側で扱う)。
export function closeSession(s: SessionTabsState, id: string): SessionTabsState {
  const idx = s.open.indexOf(id);
  if (idx === -1) return s;
  const open = s.open.filter((x) => x !== id);
  if (s.active !== id) return { open, active: s.active };
  const next = open[Math.max(0, idx - 1)] ?? '';
  return { open, active: next };
}

// タブを from から to へ移動する (ドラッグ並べ替え)。範囲外 index は no-op。
export function moveSession(s: SessionTabsState, from: number, to: number): SessionTabsState {
  if (from === to) return s;
  if (from < 0 || from >= s.open.length || to < 0 || to >= s.open.length) return s;
  const open = [...s.open];
  const [moved] = open.splice(from, 1);
  open.splice(to, 0, moved);
  return { ...s, open };
}

// 7a オーバーフロー: 隠れ側にある id を「右端の表示位置」と入替えてアクティブに
// する。既に表示域内 (index < visibleCount) ならアクティブ化のみ。
// swap 先 index は open の範囲に clamp する (visibleCount が stale でも壊れない)。
export function swapSessionToVisible(
  s: SessionTabsState,
  id: string,
  visibleCount: number,
): SessionTabsState {
  const idx = s.open.indexOf(id);
  if (idx === -1) return s;
  const lastVisible = Math.min(Math.max(1, visibleCount), s.open.length) - 1;
  if (idx <= lastVisible) return activateSession(s, id);
  const open = [...s.open];
  [open[lastVisible], open[idx]] = [open[idx], open[lastVisible]];
  return { open, active: id };
}

// localStorage 由来の値を安全な形に補正する。非文字列 / 空文字 / 重複を除去し、
// active が open に無ければ先頭 (無ければ '') に落とす。冪等。
export function normalizeSessionState(s: SessionTabsState): SessionTabsState {
  const seen = new Set<string>();
  const open: string[] = [];
  for (const id of Array.isArray(s.open) ? s.open : []) {
    if (typeof id !== 'string' || id === '' || seen.has(id)) continue;
    seen.add(id);
    open.push(id);
  }
  const active =
    typeof s.active === 'string' && open.includes(s.active) ? s.active : (open[0] ?? '');
  return { open, active };
}

// ピッカーの矢印キー移動で disabled 行をスキップした次の index を返す。
// 有効な行が無ければ -1。from が -1 (未選択) からの移動もサポートする。
export function nextEnabledIndex(
  items: ReadonlyArray<{ disabled?: boolean }>,
  from: number,
  dir: 1 | -1,
): number {
  if (items.length === 0) return -1;
  let i = from;
  for (let step = 0; step < items.length; step++) {
    i = (i + dir + items.length) % items.length;
    if (!items[i]?.disabled) return i;
  }
  return -1;
}
