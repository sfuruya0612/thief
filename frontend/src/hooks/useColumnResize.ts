// テーブル列幅のドラッグリサイズ共通フック。DataTable / DrawerDynamoItems / CostCrossTable で共用する。
// th 右端のハンドルをドラッグして列幅を変更する。初期幅は % 指定のため、ドラッグ開始時に
// 全列の実描画幅 (px) を確定させる。対象列だけを px 化すると他の列が % のまま
// 残り幅計算が不定になるため、一括してスナップショットを取り colWidths に反映する。
import { useRef, useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';

// 列幅の最小値 (px)。これより小さくはリサイズできない
const MIN_COL_WIDTH = 60;

export interface UseColumnResizeOptions {
  // ドラッグ開始時に 1 回呼ばれる (DataTable の dt-resized クラス切替等に使う)。
  onResizeStart?: () => void;
}

export function useColumnResize({ onResizeStart }: UseColumnResizeOptions = {}) {
  // ドラッグで変更した列幅 (px)。セッション内 (state) のみで保持し、永続化しない
  const [colWidths, setColWidths] = useState<Record<string, number>>({});
  const theadRowRef = useRef<HTMLTableRowElement>(null);

  const startColResize = (key: string) => (e: ReactPointerEvent<HTMLSpanElement>) => {
    e.preventDefault();
    e.stopPropagation();
    const ths = theadRowRef.current?.querySelectorAll<HTMLTableCellElement>('th[data-col-key]');
    const snapshot: Record<string, number> = {};
    ths?.forEach((th) => {
      const k = th.dataset.colKey;
      if (k) snapshot[k] = Math.round(th.getBoundingClientRect().width);
    });
    const startWidth = snapshot[key] ?? MIN_COL_WIDTH;
    const startX = e.clientX;
    setColWidths((prev) => ({ ...snapshot, ...prev }));
    onResizeStart?.();
    const move = (ev: PointerEvent) => {
      const next = Math.max(startWidth + (ev.clientX - startX), MIN_COL_WIDTH);
      setColWidths((prev) => ({ ...prev, [key]: Math.round(next) }));
    };
    const up = () => {
      document.removeEventListener('pointermove', move);
      document.removeEventListener('pointerup', up);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
    };
    document.addEventListener('pointermove', move);
    document.addEventListener('pointerup', up);
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';
  };

  return { colWidths, theadRowRef, startColResize };
}
