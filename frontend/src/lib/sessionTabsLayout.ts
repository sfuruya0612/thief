// セッションタブバーの 7a オーバーフロー計算。
// 実測ループ (タブごとの offsetWidth 収集) を避け、タブ幅を定数とみなした
// 保守的な計算にすることで、jsdom でのユニットテストと ResizeObserver
// コールバックからの単純な再計算を可能にしている。

export const SESSION_TAB_METRICS = {
  // タブ 1 本の計算上の幅。CSS は min 120px / max 150px だが、モックの
  // 「最小 120px まで縮小し名前は省略表示」に合わせ min 幅を基準にする。
  tabWidth: 120,
  gap: 2,
  // 「他 N ▾」ボタンの確保幅
  moreButtonWidth: 76,
} as const;

// availableWidth (タブ列に使える幅) に収まる表示タブ数を返す。
// 全タブが収まるなら tabCount。溢れる場合は「他 N ▾」ボタンの幅を確保した
// 上で入る本数 (最低 1、最大 tabCount - 1) を返す。
export function computeVisibleTabCount(
  availableWidth: number,
  tabCount: number,
  m: typeof SESSION_TAB_METRICS = SESSION_TAB_METRICS,
): number {
  if (tabCount <= 0) return 0;
  const per = m.tabWidth + m.gap;
  if (tabCount * per <= availableWidth) return tabCount;
  const fit = Math.floor((availableWidth - m.moreButtonWidth) / per);
  return Math.max(1, Math.min(tabCount - 1, fit));
}
