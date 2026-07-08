// tables.jsx DataTable の汎用化移植
import { useMemo, useState } from 'react';
import type { ColumnDef } from './tables/columns';

export interface DataTableProps<T extends { id: string; state?: string }> {
  rows: T[];
  columns: ColumnDef<T>[];
  onSelect: (row: T) => void;
  selectedId: string | null;
}

// ソート可能な値のみを対象にする (それ以外はソート不能として扱う)
function sortValue<T>(row: T, key: string): string | number | undefined {
  const v = (row as Record<string, unknown>)[key];
  if (typeof v === 'number' || typeof v === 'string') return v;
  if (typeof v === 'boolean') return v ? 1 : 0;
  return undefined;
}

export function DataTable<T extends { id: string; state?: string }>({
  rows,
  columns,
  onSelect,
  selectedId,
}: DataTableProps<T>) {
  const [sortKey, setSortKey] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');
  const [checked, setChecked] = useState<Set<string>>(new Set());

  const sorted = useMemo(() => {
    if (!sortKey) return rows;
    const mul = sortDir === 'asc' ? 1 : -1;
    return [...rows].sort((a, b) => {
      const av = sortValue(a, sortKey);
      const bv = sortValue(b, sortKey);
      if (av == null) return 1;
      if (bv == null) return -1;
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * mul;
      return String(av).localeCompare(String(bv)) * mul;
    });
  }, [rows, sortKey, sortDir]);

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

  return (
    <div className="table-wrap">
      <table className="dt">
        <colgroup>
          <col style={{ width: 32 }} />
          {columns.map((c) => (
            <col key={c.key} style={{ width: c.width }} />
          ))}
        </colgroup>
        <thead>
          <tr>
            <th>
              <input
                type="checkbox"
                className="cb"
                checked={checked.size === rows.length && rows.length > 0}
                onChange={(e) =>
                  setChecked(e.target.checked ? new Set(rows.map((r) => r.id)) : new Set())
                }
              />
            </th>
            {columns.map((c) => (
              <th
                key={c.key}
                className={`sortable ${sortKey === c.key ? 'sorted' : ''}`}
                style={{ textAlign: c.align ?? 'left' }}
                onClick={() => toggleSort(c.key)}
              >
                {c.header}
                <span className="sort">
                  {sortKey === c.key ? (sortDir === 'asc' ? '▲' : '▼') : '▲▼'}
                </span>
              </th>
            ))}
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
