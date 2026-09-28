// tables.jsx DataTable の汎用化移植
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { filterText, type ColumnDef } from './tables/columns';
import { Loading } from './Loading';
import { useColumnResize } from '../hooks/useColumnResize';

// DataTableSelection は行のチェックボックス列を外から制御するための props。
// 渡した一覧では選択状態を呼び出し側が持ち、渡さない一覧は従来どおり内部 state を使う。
export interface DataTableSelection<T> {
  selected: ReadonlySet<string>;
  onChange: (next: Set<string>) => void;
  // false の行にはチェックボックスを描かない。
  isSelectable: (row: T) => boolean;
}

export interface DataTableProps<T extends { id: string; state?: string }> {
  rows: T[];
  columns: ColumnDef<T>[];
  onSelect: (row: T) => void;
  selectedId: string | null;
  isLoading?: boolean;
  // rowClassName は行ごとに追加する CSS クラスを返す (未指定なら追加しない)。
  // オブジェクトブラウザのプレビュー不可行のグレーアウト等に使う。
  rowClassName?: (row: T) => string | undefined;
  // selection を渡すとチェックボックス列を制御化する (オブジェクトブラウザの複数選択)。
  selection?: DataTableSelection<T>;
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

export function DataTable<T extends { id: string; state?: string }>({
  rows,
  columns,
  onSelect,
  selectedId,
  isLoading,
  rowClassName,
  selection,
}: DataTableProps<T>) {
  const { t } = useTranslation('app');
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

  // 制御化したチェックボックス (selection) の対象行。列フィルタとソートを適用した表示順の
  // isSelectable な行 (「選べる行」) がヘッダの全選択・全解除の対象になる。
  const selectedSet = selection?.selected;
  const isSelectable = selection?.isSelectable;
  const selectableRows = useMemo(
    () => (isSelectable ? sorted.filter((r) => isSelectable(r)) : []),
    [isSelectable, sorted],
  );
  const selectedSelectableCount = useMemo(
    () => (selectedSet ? selectableRows.filter((r) => selectedSet.has(r.id)).length : 0),
    [selectableRows, selectedSet],
  );
  const headerChecked = selection
    ? selectableRows.length > 0 && selectedSelectableCount === selectableRows.length
    : checked.size === filtered.length && filtered.length > 0;
  const headerIndeterminate = selection ? selectedSelectableCount > 0 && !headerChecked : false;
  // indeterminate は HTML の属性ではなく DOM のプロパティのため、ref で反映する。
  const headerCheckboxRef = useCallback(
    (el: HTMLInputElement | null) => {
      if (el) el.indeterminate = headerIndeterminate;
    },
    [headerIndeterminate],
  );

  // ヘッダのチェックボックス。制御化した一覧では「選べる行を 1 つも選んでいなければ全選択、
  // 1 つでも選んでいれば全解除」にする。indeterminate からのクリックが全解除になるため、
  // 選択の打ち切りで全件を選べない一覧でも全解除に到達できる。
  const toggleAllChecked = (checkedByClick: boolean) => {
    if (selection) {
      selection.onChange(
        selectedSelectableCount === 0 ? new Set(selectableRows.map((r) => r.id)) : new Set(),
      );
      return;
    }
    setChecked(checkedByClick ? new Set(filtered.map((r) => r.id)) : new Set());
  };

  const toggleRow = (id: string, next: boolean) => {
    if (selection) {
      const nextSet = new Set(selection.selected);
      if (next) nextSet.add(id);
      else nextSet.delete(id);
      selection.onChange(nextSet);
      return;
    }
    toggleRowChecked(id, next);
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
              {/* 制御化した一覧で選べる行が 1 つも表示されていなければ、ヘッダのチェックボックスは
                  出さない (ヘッダの対象は表示中の選べる行で、対象が無いため)。列フィルタで隠れた
                  行の選択は残り、フィルタを消せばヘッダから全解除できる。 */}
              {(!selection || selectableRows.length > 0) && (
                <input
                  ref={headerCheckboxRef}
                  type="checkbox"
                  className="cb"
                  checked={headerChecked}
                  onChange={(e) => toggleAllChecked(e.target.checked)}
                />
              )}
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
                    placeholder={t('dataTable.filterPlaceholder')}
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
              className={[selectedId === r.id ? 'selected' : '', rowClassName?.(r) ?? '']
                .filter(Boolean)
                .join(' ')}
              onClick={() => onSelect(r)}
            >
              <td onClick={(e) => e.stopPropagation()}>
                {(!selection || selection.isSelectable(r)) && (
                  <input
                    type="checkbox"
                    className="cb"
                    checked={selection ? selection.selected.has(r.id) : checked.has(r.id)}
                    onChange={(e) => toggleRow(r.id, e.target.checked)}
                  />
                )}
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
