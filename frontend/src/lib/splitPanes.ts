// 分割表示 (2 ペイン) の状態と遷移の純関数群。フック (hooks/useSplitPanes.ts) が
// useReducer で包み、App.tsx がビューごとに 1 つずつ持つ。
// 変化の無い遷移は引数と同じ参照を返し、React の再描画を抑える
// (components/Terminal/terminalDockStatuses.ts の pruneStatuses と同じ扱い)。

export interface SplitPanesState {
  // 各ペインが表示中のサービス。長さは 1 (分割なし) または 2 (分割中) で、
  // null はサービス未選択のペインを表す。
  services: (string | null)[];
  // ペインごとに固定の識別子 (services と同じ長さ)。React の key に使う。添字を key に
  // すると、左のペインを閉じて右のペインが添字 0 へ移ったときに、閉じたペインの状態を
  // 残ったペインが引き継いでしまう。識別子を保つことでこれを防ぐ。
  ids: number[];
  // フォーカス中のペインの添字。サイドバーの選択と ESC の受け取り先になる。
  focused: number;
}

// サービスを選択する。他のペインが表示中のサービスなら、フォーカスをそのペインへ
// 移すだけにして同じサービスの二重表示を防ぐ (Athena / BigQuery のエディタタブは
// サービスとスコープ単位の localStorage キーを共有するため)。それ以外は
// フォーカス中のペインのサービスを置き換える。
export function selectService(state: SplitPanesState, service: string): SplitPanesState {
  if (state.focused < 0 || state.focused >= state.services.length) return state;
  const other = state.services.findIndex((s, i) => i !== state.focused && s === service);
  if (other !== -1) return { ...state, focused: other };
  if (state.services[state.focused] === service) return state;
  const services = state.services.map((s, i) => (i === state.focused ? service : s));
  return { ...state, services };
}

// 分割を開始する。2 つ目のペインをサービス未選択 (null) で追加し、フォーカスを
// 2 つ目へ移す。サービス未選択から始めるのは、1 つ目と同じサービスの二重表示を
// 禁じているため、初期サービスを規則で選べないからである。
export function openSplit(state: SplitPanesState): SplitPanesState {
  if (state.services.length !== 1) return state;
  // 既存の識別子と重ならない値にする (Math.max は空配列で -Infinity になるため長さで分ける)
  const nextId = state.ids.length === 0 ? 0 : Math.max(...state.ids) + 1;
  return { services: [...state.services, null], ids: [...state.ids, nextId], focused: 1 };
}

// ペインを閉じて 1 ペインに戻す。残ったペインの識別子は保つ (React の key に使うため)。
// 残ったペインのフォーカスは 0 になる。1 ペインのときと範囲外の添字は同じ状態を返す。
export function closePane(state: SplitPanesState, index: number): SplitPanesState {
  if (state.services.length !== 2) return state;
  if (index !== 0 && index !== 1) return state;
  return { services: [state.services[1 - index]], ids: [state.ids[1 - index]], focused: 0 };
}

// ペインのフォーカスを移す。範囲外の添字と同じペインは同じ状態を返す。
export function focusPane(state: SplitPanesState, index: number): SplitPanesState {
  if (index < 0 || index >= state.services.length || index === state.focused) return state;
  return { ...state, focused: index };
}
