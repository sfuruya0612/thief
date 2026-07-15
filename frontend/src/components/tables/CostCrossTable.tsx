// Cost Explorer 専用のクロス集計テーブル。
// 縦軸に GroupBy の集計結果 (サービス名等)、横軸に日付ごとの費用を並べる。
// 列数が日付範囲に応じて可変なため、ColumnDef<T>[] 前提の DataTable では表現できず専用実装とする。
// 列幅ドラッグリサイズは DataTable.tsx の startColResize と同型のロジックを移植する。
import { useRef, useState } from 'react';

export interface CostCrossTableRow {
  group: string;
  amounts: number[];
  total: number;
}

export interface CostCrossTableProps {
  categories: string[];
  rows: CostCrossTableRow[];
}

function formatAmount(v: number): string {
  return `$${v.toLocaleString(undefined, { maximumFractionDigits: 2 })}`;
}

// 列幅の最小値 (px)。これより小さくはリサイズできない
const MIN_COL_WIDTH = 60;
// リサイズ前のデフォルト幅。以前の CSS 固定値 (160px / 88px) と同じ見た目になるよう揃える
const DEFAULT_GROUP_WIDTH = 160;
const DEFAULT_TOTAL_WIDTH = 88;
const DEFAULT_CATEGORY_WIDTH = 88;

export function CostCrossTable({ categories, rows }: CostCrossTableProps) {
  // ドラッグで変更した列幅 (px)。キーは 'group' / 'total' / 日付 (category) 文字列
  const [colWidths, setColWidths] = useState<Record<string, number>>({});
  const theadRowRef = useRef<HTMLTableRowElement>(null);

  // Group 列の幅は Total 列の sticky left オフセットに使うため先に確定する
  const groupWidth = colWidths.group ?? DEFAULT_GROUP_WIDTH;
  const totalWidth = colWidths.total ?? DEFAULT_TOTAL_WIDTH;

  // th 右端のハンドルをドラッグして列幅を変更する (DataTable.tsx の startColResize と同型)。
  // Group 列の幅が変わると Total 列の sticky left オフセットもずれるため、ドラッグ開始時に
  // 全列の実描画幅を px で確定してから対象列のみ更新する
  const startColResize = (key: string) => (e: React.PointerEvent<HTMLSpanElement>) => {
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

  return (
    <div className="table-wrap">
      <table className="dt cost-cross-table">
        <colgroup>
          <col style={{ width: groupWidth }} />
          <col style={{ width: totalWidth }} />
          {categories.map((c) => (
            <col key={c} style={{ width: colWidths[c] ?? DEFAULT_CATEGORY_WIDTH }} />
          ))}
        </colgroup>
        <thead>
          <tr ref={theadRowRef}>
            <th data-col-key="group" style={{ textAlign: 'left', left: 0 }}>
              Group
              <span
                className="col-resize-handle"
                onPointerDown={startColResize('group')}
                title="Drag to resize column"
              />
            </th>
            <th data-col-key="total" style={{ textAlign: 'right', left: groupWidth }}>
              Total
              <span
                className="col-resize-handle"
                onPointerDown={startColResize('total')}
                title="Drag to resize column"
              />
            </th>
            {categories.map((c) => (
              <th key={c} data-col-key={c} style={{ textAlign: 'right' }}>
                {c}
                <span
                  className="col-resize-handle"
                  onPointerDown={startColResize(c)}
                  title="Drag to resize column"
                />
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.group}>
              <td className="primary truncate" style={{ left: 0 }}>
                {r.group}
              </td>
              <td
                style={{
                  textAlign: 'right',
                  fontFamily: 'var(--font-mono)',
                  fontWeight: 600,
                  left: groupWidth,
                }}
              >
                {formatAmount(r.total)}
              </td>
              {r.amounts.map((a, i) => (
                <td
                  key={categories[i]}
                  style={{ textAlign: 'right', fontFamily: 'var(--font-mono)' }}
                >
                  {formatAmount(a)}
                </td>
              ))}
            </tr>
          ))}
          {rows.length === 0 && (
            <tr>
              <td
                colSpan={categories.length + 2}
                style={{ textAlign: 'center', padding: 40, color: 'var(--text-3)' }}
              >
                No cost data match current filters
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
