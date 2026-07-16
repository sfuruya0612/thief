// tables.jsx DataTable の汎用化移植
import { useMemo, useState } from 'react';
import type { ColumnDef } from './tables/columns';
import { Loading } from './Loading';
import { useColumnResize } from '../hooks/useColumnResize';

export interface DataTableProps<T extends { id: string; state?: string }> {
  rows: T[];
  columns: ColumnDef<T>[];
  onSelect: (row: T) => void;
  selectedId: string | null;
  isLoading?: boolean;
}

// ソート可能な値のみを対象にする (それ以外はソート不能として扱う)
function sortValue<T>(row: T, key: string): string | number | undefined {
  const v = (row as Record<string, unknown>)[key];
  if (typeof v === 'number' || typeof v === 'string') return v;
  if (typeof v === 'boolean') return v ? 1 : 0;
  return undefined;
}

// 列フィルターの対象にするかどうか。明示指定を優先し、未指定時は
// header が空 (Actions 列等) でも key === 'actions' でもない列を対象とする
function isFilterable<T>(c: ColumnDef<T>): boolean {
  return c.filterable ?? (c.header !== '' && c.key !== 'actions');
}

// 列フィルターの判定に使う文字列を取り出す。filterValue 指定があればそれを使い、
// なければ row[key] の生値を文字列化する (cell の表示値とは異なる場合がある)
function filterText<T>(c: ColumnDef<T>, row: T): string {
  if (c.filterValue) return c.filterValue(row);
  const v = (row as Record<string, unknown>)[c.key];
  return v == null ? '' : String(v);
}

export function DataTable<T extends { id: string; state?: string }>({
  rows,
  columns,
  onSelect,
  selectedId,
  isLoading,
}: DataTableProps<T>) {
  const [sortKey, setSortKey] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');
  const [checked, setChecked] = useState<Set<string>>(new Set());
  // 列リサイズが一度でも行われたか。true になると table を colgroup の px 合計幅で
  // 描画し、はみ出した分は .table-wrap の横スクロールに委ねる (dt-resized クラス)
  const [resized, setResized] = useState(false);
  const { colWidths, theadRowRef, startColResize } = useColumnResize({
    onResizeStart: () => setResized(true),
  });
  // 列ごとのフィルター入力値 (key -> 入力文字列)
  const [colFilters, setColFilters] = useState<Record<string, string>>({});

  const filtered = useMemo(() => {
    const activeCols = columns.filter((c) => isFilterable(c) && colFilters[c.key]?.trim());
    if (activeCols.length === 0) return rows;
    return rows.filter((row) =>
      activeCols.every((c) =>
        filterText(c, row).toLowerCase().includes(colFilters[c.key].trim().toLowerCase()),
      ),
    );
  }, [rows, columns, colFilters]);

  const sorted = useMemo(() => {
    if (!sortKey) return filtered;
    const mul = sortDir === 'asc' ? 1 : -1;
    return [...filtered].sort((a, b) => {
      const av = sortValue(a, sortKey);
      const bv = sortValue(b, sortKey);
      if (av == null) return 1;
      if (bv == null) return -1;
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * mul;
      return String(av).localeCompare(String(bv)) * mul;
    });
  }, [filtered, sortKey, sortDir]);

  const toggleSort = (k: string) => {
    if (sortKey === k) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortKey(k);
      setSortDir('asc');
    }
  };

  const toggleRowChecked = (id: string, next: boolean) => {
    setChecked((prev) => {
      const n = new Set(prev);
      if (next) n.add(id);
      else n.delete(id);
      return n;
    });
  };

  if (isLoading) {
    return (
      <div className="table-wrap">
        <Loading />
      </div>
    );
  }

  return (
    <div className="table-wrap">
      <table className={`dt${resized ? ' dt-resized' : ''}`}>
        <colgroup>
          <col style={{ width: 32 }} />
          {columns.map((c) => (
            <col key={c.key} style={{ width: colWidths[c.key] ?? c.width }} />
          ))}
        </colgroup>
        <thead>
          <tr ref={theadRowRef}>
            <th>
              <input
                type="checkbox"
                className="cb"
                checked={checked.size === filtered.length && filtered.length > 0}
                onChange={(e) =>
                  setChecked(e.target.checked ? new Set(filtered.map((r) => r.id)) : new Set())
                }
              />
            </th>
            {columns.map((c) => (
              <th
                key={c.key}
                data-col-key={c.key}
                className={`sortable ${sortKey === c.key ? 'sorted' : ''}`}
                style={{ textAlign: c.align ?? 'left', position: 'relative' }}
                onClick={() => toggleSort(c.key)}
              >
                {c.header}
                <span className="sort">
                  {sortKey === c.key ? (sortDir === 'asc' ? '▲' : '▼') : '▲▼'}
                </span>
                <span
                  className="col-resize-handle"
                  onPointerDown={startColResize(c.key)}
                  title="Drag to resize column"
                />
              </th>
            ))}
          </tr>
          <tr className="dt-filter-row">
            <th />
            {columns.map((c) =>
              isFilterable(c) ? (
                <th key={c.key}>
                  <input
                    className="dt-col-filter"
                    value={colFilters[c.key] ?? ''}
                    placeholder="フィルター…"
                    onClick={(e) => e.stopPropagation()}
                    onChange={(e) =>
                      setColFilters((prev) => ({ ...prev, [c.key]: e.target.value }))
                    }
                  />
                </th>
              ) : (
                <th key={c.key} />
              ),
            )}
          </tr>
        </thead>
        <tbody>
          {sorted.map((r) => (
            <tr
              key={r.id}
              className={selectedId === r.id ? 'selected' : ''}
              onClick={() => onSelect(r)}
            >
              <td onClick={(e) => e.stopPropagation()}>
                <input
                  type="checkbox"
                  className="cb"
                  checked={checked.has(r.id)}
                  onChange={(e) => toggleRowChecked(r.id, e.target.checked)}
                />
              </td>
              {columns.map((c) => (
                <td key={c.key} style={{ textAlign: c.align ?? 'left' }}>
                  {c.cell(r)}
                </td>
              ))}
            </tr>
          ))}
          {sorted.length === 0 && (
            <tr>
              <td
                colSpan={columns.length + 1}
                style={{ textAlign: 'center', padding: 40, color: 'var(--text-3)' }}
              >
                No resources match current filters
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
