// 分割表示 (2 ペイン) の状態を useReducer で保持するフック。遷移は
// lib/splitPanes.ts の純関数に集約し、ここは dispatch の薄いラッパーに徹する。
// App.tsx が AWS 用と Google Cloud 用に 1 つずつ呼ぶ (ペインの状態はビューごとに持つ)。
import { useCallback, useReducer } from 'react';
import {
  closePane,
  focusPane,
  openSplit,
  selectService,
  type SplitPanesState,
} from '../lib/splitPanes';

type SplitPanesAction =
  | { type: 'selectService'; service: string }
  | { type: 'openSplit' }
  | { type: 'closePane'; index: number }
  | { type: 'focusPane'; index: number };

function reducer(state: SplitPanesState, action: SplitPanesAction): SplitPanesState {
  switch (action.type) {
    case 'selectService':
      return selectService(state, action.service);
    case 'openSplit':
      return openSplit(state);
    case 'closePane':
      return closePane(state, action.index);
    case 'focusPane':
      return focusPane(state, action.index);
  }
}

export interface SplitPanesApi {
  panes: SplitPanesState;
  selectService: (service: string) => void;
  openSplit: () => void;
  closePane: (index: number) => void;
  focusPane: (index: number) => void;
}

// initialService は 1 ペイン目の初期サービス (サービス未選択を許すため string | null)。
export function useSplitPanes(initialService: string | null): SplitPanesApi {
  const [panes, dispatch] = useReducer(reducer, initialService, (service) => ({
    services: [service],
    ids: [0],
    focused: 0,
  }));

  const select = useCallback((service: string) => dispatch({ type: 'selectService', service }), []);
  const open = useCallback(() => dispatch({ type: 'openSplit' }), []);
  const close = useCallback((index: number) => dispatch({ type: 'closePane', index }), []);
  const focus = useCallback((index: number) => dispatch({ type: 'focusPane', index }), []);

  return { panes, selectService: select, openSplit: open, closePane: close, focusPane: focus };
}
