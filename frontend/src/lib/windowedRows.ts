// 単価表 (RateGroupSection) の手書き windowing 用の可視範囲計算。
// スクロールは Pricing パネル全体を包む単一領域 (.pr-stack) で発生し、各グループの
// テーブルはその中に積まれる (components/pricing/RateGroupSection.tsx 参照)。
// そのため各テーブルは「スクロール祖先の scrollTop / 高さ」と「自テーブル先頭の
// スクロールコンテンツ座標系での位置」から、自分が描画すべき行範囲を割り出す。
//
// DOM 依存 (要素の測定や scroll 購読) はフック側 (hooks/useWindowedRows.ts) に置き、
// ここは純粋な算術のみに保つ。jsdom はレイアウトを持たない (getBoundingClientRect や
// clientHeight が 0) ため、可視範囲のロジックはこの純関数を単体テストで担保する。

// WindowRange は描画すべき行の半開区間 [start, end) と、その前後に挿入する
// スペーサーの高さ (px) を表す。
export interface WindowRange {
  // 描画を開始する行インデックス (0 始まり、含む)。
  start: number;
  // 描画を終了する行インデックス (含まない)。start <= end <= rowCount。
  end: number;
  // [0, start) 行分の高さ。テーブル先頭のスペーサー行に与える。
  topPad: number;
  // [end, rowCount) 行分の高さ。テーブル末尾のスペーサー行に与える。
  bottomPad: number;
}

// ComputeVisibleRangeParams は computeVisibleRange の入力。
export interface ComputeVisibleRangeParams {
  // スクロール祖先の scrollTop。
  scrollTop: number;
  // スクロール祖先の可視領域の高さ (clientHeight)。
  viewportHeight: number;
  // 行リスト先頭の、スクロールコンテンツ座標系での上端位置。
  // = (リスト先頭要素の viewport 上端) - (スクロール祖先の viewport 上端) + scrollTop。
  listTop: number;
  // 1 行の高さ (px)。グループ内は同一 model で均一なため単一値で扱う。
  rowHeight: number;
  // 行の総数。
  rowCount: number;
  // 可視範囲の上下に余分に描画する行数。スクロール中の未描画のちらつきを防ぐ。
  overscan: number;
}

// clamp は value を [min, max] に収める。min > max のときは min を返す。
function clamp(value: number, min: number, max: number): number {
  if (value < min) return min;
  if (value > max) return max;
  return value;
}

// computeVisibleRange は可視領域と行高から、描画すべき行範囲とスペーサー高さを求める。
//
// rowHeight が 0 以下、rowCount が 0 以下、または非有限な入力の場合は「全行を描画する」
// フォールバック (windowing 無効) を返す。これは行高の実測が済むまでの初期状態や、
// 想定外の測定値に対する安全側の挙動である。
export function computeVisibleRange(params: ComputeVisibleRangeParams): WindowRange {
  const { scrollTop, viewportHeight, listTop, rowHeight, rowCount, overscan } = params;

  if (rowCount <= 0) {
    return { start: 0, end: 0, topPad: 0, bottomPad: 0 };
  }

  const finite =
    Number.isFinite(scrollTop) &&
    Number.isFinite(viewportHeight) &&
    Number.isFinite(listTop) &&
    Number.isFinite(rowHeight);
  if (!finite || rowHeight <= 0 || viewportHeight <= 0) {
    // windowing 無効 (全描画)。スペーサーは不要。
    return { start: 0, end: rowCount, topPad: 0, bottomPad: 0 };
  }

  const safeOverscan = Number.isFinite(overscan) && overscan > 0 ? Math.floor(overscan) : 0;

  // 可視領域をリスト先頭からの相対座標に変換する。
  const relTop = scrollTop - listTop;
  const relBottom = relTop + viewportHeight;

  // relBottom <= 0: リストはビューポートより下にあり 1 行も見えない。
  // relTop >= リスト全体の高さ: リストはビューポートより上に通り過ぎた。
  // いずれも clamp で start/end が縮退し、適切なスペーサーだけが残る。
  const first = Math.floor(relTop / rowHeight) - safeOverscan;
  const last = Math.ceil(relBottom / rowHeight) + safeOverscan;

  const start = clamp(first, 0, rowCount);
  const end = clamp(last, start, rowCount);

  return {
    start,
    end,
    topPad: start * rowHeight,
    bottomPad: (rowCount - end) * rowHeight,
  };
}
