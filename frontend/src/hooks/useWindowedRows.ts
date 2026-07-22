// 単価表 (RateGroupSection) の手書き windowing を DOM に配線するフック。
// 可視範囲の算術は lib/windowedRows.ts の純関数 computeVisibleRange に委ね、ここは
// 「スクロール祖先の発見」「行高の実測」「scroll / resize の購読」「範囲 state の更新」
// だけを担う。
//
// Pricing 画面の縦スクロールは複数の ServiceCard/グループを包む単一領域 (.pr-stack) で
// 発生する。あるグループが高さを変える (フィルタ・条件セレクタ・折りたたみ) と、同じ
// スクロール領域に積まれた下のグループの位置がずれるが、これは scroll イベントを伴わない。
// そのため、スクロール領域ごとに共有 ResizeObserver を 1 個持ち、いずれかのグループの
// サイズ変化で同じ領域の全グループを再計算する。これにより sibling のレイアウト変化にも
// 追従する。
import { useCallback, useLayoutEffect, useRef, useState } from 'react';
import { computeVisibleRange, type WindowRange } from '../lib/windowedRows';

// isScrollable は overflow-y がスクロールを生む値かを判定する。
function isScrollable(el: Element): boolean {
  const oy = getComputedStyle(el).overflowY;
  return oy === 'auto' || oy === 'scroll' || oy === 'overlay';
}

// findScrollParent は要素から上に辿って最初のスクロール可能な祖先を返す。
// 見つからなければ null (windowing は無効化され全描画にフォールバックする)。
function findScrollParent(el: Element | null): HTMLElement | null {
  let cur = el?.parentElement ?? null;
  while (cur) {
    if (isScrollable(cur)) return cur;
    cur = cur.parentElement;
  }
  return null;
}

// スクロール領域ごとの購読レジストリ。同一スクロール領域に属する全グループの再計算
// コールバックを 1 つの ResizeObserver と 1 つの scroll リスナで束ねる。
interface ContainerRegistry {
  callbacks: Set<() => void>;
  observer: ResizeObserver;
  onScroll: () => void;
  // rAF スロットル用。連続する scroll / resize を 1 フレームに集約する。
  rafId: number | null;
}

const registries = new WeakMap<HTMLElement, ContainerRegistry>();

// getRegistry は指定スクロール領域のレジストリを取得 (なければ生成) する。
function getRegistry(scrollEl: HTMLElement): ContainerRegistry {
  const existing = registries.get(scrollEl);
  if (existing) return existing;

  const reg: ContainerRegistry = {
    callbacks: new Set(),
    rafId: null,
    onScroll: () => {},
    // ダミー。直後に本体へ差し替える。
    observer: new ResizeObserver(() => {}),
  };

  const fireAll = () => {
    if (reg.rafId !== null) return;
    reg.rafId = requestAnimationFrame(() => {
      reg.rafId = null;
      // コールバックのコピーを回す (実行中の登録解除に対して安全)。
      for (const cb of [...reg.callbacks]) cb();
    });
  };

  reg.onScroll = fireAll;
  reg.observer = new ResizeObserver(fireAll);
  reg.observer.observe(scrollEl);
  scrollEl.addEventListener('scroll', reg.onScroll, { passive: true });
  registries.set(scrollEl, reg);
  return reg;
}

// releaseRegistry はコールバックを登録解除し、空になったら scroll 領域の購読を破棄する。
function releaseRegistry(scrollEl: HTMLElement, cb: () => void): void {
  const reg = registries.get(scrollEl);
  if (!reg) return;
  reg.callbacks.delete(cb);
  if (reg.callbacks.size > 0) return;
  scrollEl.removeEventListener('scroll', reg.onScroll);
  reg.observer.disconnect();
  if (reg.rafId !== null) cancelAnimationFrame(reg.rafId);
  registries.delete(scrollEl);
}

// 初回描画で仮定するビューポート高さ (px)。実際の高さは scrollEl の clientHeight を
// 測って確定するが、初回はまだ DOM が無いためこの控えめな値で描画行数を抑える。過大でも
// 直後の layout effect が paint 前に補正するため、初回に生成する行数を減らす効果だけを狙う。
const INITIAL_VIEWPORT_GUESS = 1000;

// UseWindowedRowsParams はフックの入力。
export interface UseWindowedRowsParams {
  // 行の総数。
  rowCount: number;
  // false のとき windowing を無効化し、全行を描画する範囲を返す (小さいリスト向け)。
  enabled: boolean;
  // 行高の初期推定値 (px)。実測が済むまでこの値で範囲を計算するため、初回描画から
  // windowing が効く。実測後は測定値へ補正される。
  estimateRowHeight: number;
  // 可視範囲の上下に余分に描画する行数。
  overscan?: number;
}

// UseWindowedRowsResult はフックの返り値。
export interface UseWindowedRowsResult {
  // 描画すべき行範囲とスペーサー高さ。
  range: WindowRange;
  // グループのルート要素 (.pr-group) に付ける ref。サイズ変化の監視対象。
  rootRef: (el: HTMLElement | null) => void;
  // 行リスト先頭要素 (tbody) に付ける ref。リスト位置の測定に使う。
  listRef: (el: HTMLElement | null) => void;
  // 可視範囲の先頭データ行に付ける ref。行高の実測に使う。
  rowRef: (el: HTMLElement | null) => void;
}

const DEFAULT_OVERSCAN = 8;

// useWindowedRows は可視範囲とスペーサー高さ、および測定用の ref 群を返す。
export function useWindowedRows(params: UseWindowedRowsParams): UseWindowedRowsResult {
  const { rowCount, enabled, estimateRowHeight, overscan = DEFAULT_OVERSCAN } = params;

  const rootElRef = useRef<HTMLElement | null>(null);
  const listElRef = useRef<HTMLElement | null>(null);
  const scrollElRef = useRef<HTMLElement | null>(null);
  const rowHeightRef = useRef<number>(estimateRowHeight);
  // rowRef で受け取った先頭データ行の実測高さ。0 は未測定。
  const measuredRowHeightRef = useRef<number>(0);

  const [range, setRange] = useState<WindowRange>(() => {
    if (!enabled) return { start: 0, end: rowCount, topPad: 0, bottomPad: 0 };
    // 初回は DOM が無いため、推定行高と控えめなビューポートで先頭付近だけを描画する。
    // 位置がずれていても直後の layout effect が paint 前に正しい範囲へ補正する。
    return computeVisibleRange({
      scrollTop: 0,
      viewportHeight: INITIAL_VIEWPORT_GUESS,
      listTop: 0,
      rowHeight: estimateRowHeight,
      rowCount,
      overscan,
    });
  });

  // recompute は現在の測定値から範囲を求めて state を更新する。値が変わらなければ
  // 再レンダーを避ける。
  const recompute = useCallback(() => {
    const scrollEl = scrollElRef.current;
    const listEl = listElRef.current;
    if (!enabled || !scrollEl || !listEl) return;

    const scrollRect = scrollEl.getBoundingClientRect();
    const listRect = listEl.getBoundingClientRect();
    // tbody の上端は論理的な行 0 の位置に一致する (先頭スペーサーが行 [0, start) を占める)。
    const listTop = listRect.top - scrollRect.top + scrollEl.scrollTop;

    const next = computeVisibleRange({
      scrollTop: scrollEl.scrollTop,
      viewportHeight: scrollEl.clientHeight,
      listTop,
      rowHeight: rowHeightRef.current,
      rowCount,
      overscan,
    });

    setRange((prev) =>
      prev.start === next.start &&
      prev.end === next.end &&
      prev.topPad === next.topPad &&
      prev.bottomPad === next.bottomPad
        ? prev
        : next,
    );
  }, [enabled, rowCount, overscan]);

  // スクロール領域の発見と購読登録。enabled / rowCount / recompute の変化で貼り直す。
  useLayoutEffect(() => {
    if (!enabled) {
      setRange({ start: 0, end: rowCount, topPad: 0, bottomPad: 0 });
      return;
    }
    const scrollEl = findScrollParent(listElRef.current);
    scrollElRef.current = scrollEl;
    if (!scrollEl) {
      // スクロール祖先が無ければ windowing を無効化して全描画する。
      setRange({ start: 0, end: rowCount, topPad: 0, bottomPad: 0 });
      return;
    }

    const reg = getRegistry(scrollEl);
    reg.callbacks.add(recompute);
    const observedRoot = rootElRef.current;
    if (observedRoot) reg.observer.observe(observedRoot);

    recompute();

    return () => {
      if (observedRoot) reg.observer.unobserve(observedRoot);
      releaseRegistry(scrollEl, recompute);
    };
  }, [enabled, rowCount, recompute]);

  // 実測した行高で範囲を補正する。先頭データ行の高さ (rowRef で測定) が推定値と異なれば
  // 反映して再計算する。毎レンダー後に走るが、値が収束すれば recompute は呼ばれない。
  useLayoutEffect(() => {
    if (!enabled) return;
    const measured = measuredRowHeightRef.current;
    if (measured > 0 && measured !== rowHeightRef.current) {
      rowHeightRef.current = measured;
      recompute();
    }
  });

  const rootRef = useCallback((el: HTMLElement | null) => {
    rootElRef.current = el;
  }, []);

  const listRef = useCallback((el: HTMLElement | null) => {
    listElRef.current = el;
  }, []);

  const rowRef = useCallback((el: HTMLElement | null) => {
    if (el) measuredRowHeightRef.current = el.offsetHeight;
  }, []);

  return { range, rootRef, listRef, rowRef };
}
