// クエリ結果テーブル。列ヘッダクリックでソート、列ごとの部分一致フィルター、
// クライアントサイドページング (50 行/ページ)、未取得ページの追加読み込みを提供する。
import { useMemo, useState, type ReactNode } from 'react';

export interface ResultTableProps {
  columns: string[];
  rows: string[][];
  // サーバ側の総行数 (BigQuery)。未取得行がある場合の表示に使う
  totalRows?: number;
  hasMore?: boolean;
  isFetchingMore?: boolean;
  onLoadMore?: () => void;
  footerRight?: ReactNode;
}

const PAGE_SIZE = 50;

// 数値セル判定 (カンマ区切りを許容)
function isNumericCell(v: string): boolean {
  return /^-?[\d,]+(\.\d+)?$/.test(v);
}

function parseNumericCell(v: string): number {
  const n = Number(v.replace(/,/g, ''));
  return Number.isFinite(n) ? n : 0;
}

export function ResultTable({
  columns,
  rows,
  totalRows,
  hasMore,
  isFetchingMore,
  onLoadMore,
  footerRight,
}: ResultTableProps) {
  const [sort, setSort] = useState<{ col: number; dir: 'asc' | 'desc' } | null>(null);
  const [filters, setFilters] = useState<Record<number, string>>({});
  const [page, setPage] = useState(0);

  // 全セルが数値として解釈できる列は数値ソート + 右寄せにする
  const numericCols = useMemo(
    () =>
      columns.map(
        (_, i) =>
          rows.length > 0 &&
          rows.every((r) => (r[i] ?? '') === '' || isNumericCell(r[i] ?? '')) &&
          rows.some((r) => (r[i] ?? '') !== ''),
      ),
    [columns, rows],
  );

  const filtered = useMemo(() => {
    const active = Object.entries(filters).filter(([, v]) => v.trim() !== '');
    if (active.length === 0) return rows;
    return rows.filter((r) =>
      active.every(([i, v]) => (r[Number(i)] ?? '').toLowerCase().includes(v.trim().toLowerCase())),
    );
  }, [rows, filters]);

  const sorted = useMemo(() => {
    if (!sort) return filtered;
    const { col, dir } = sort;
    const mul = dir === 'asc' ? 1 : -1;
    return [...filtered].sort((a, b) => {
      const av = a[col] ?? '';
      const bv = b[col] ?? '';
      if (numericCols[col]) return (parseNumericCell(av) - parseNumericCell(bv)) * mul;
      return av.localeCompare(bv) * mul;
    });
  }, [filtered, sort, numericCols]);

  const pageCount = Math.max(1, Math.ceil(sorted.length / PAGE_SIZE));
  const clampedPage = Math.min(page, pageCount - 1);
  const start = clampedPage * PAGE_SIZE;
  const pageRows = sorted.slice(start, start + PAGE_SIZE);

  const toggleSort = (col: number) => {
    setSort((prev) =>
      prev?.col === col ? { col, dir: prev.dir === 'asc' ? 'desc' : 'asc' } : { col, dir: 'asc' },
    );
  };

  const rangeLabel =
    sorted.length === 0
      ? '0 行'
      : `${sorted.length.toLocaleString()} 行中 ${(start + 1).toLocaleString()}–${(
          start + pageRows.length
        ).toLocaleString()} を表示`;
  const totalLabel =
    totalRows !== undefined && totalRows > rows.length
      ? ` (全 ${totalRows.toLocaleString()} 行)`
      : '';

  return (
    <div className="qe-rt">
      <div className="qe-rt-scroll">
        <table className="qe-rt-table">
          <thead>
            <tr>
              <th className="qe-rt-idx">#</th>
              {columns.map((c, i) => (
                <th
                  key={i}
                  className={numericCols[i] ? 'num' : ''}
                  onClick={() => toggleSort(i)}
                  title={c}
                >
                  {c}{' '}
                  <span className={`qe-rt-sort ${sort?.col === i ? 'active' : ''}`}>
                    {sort?.col === i ? (sort.dir === 'asc' ? '▲' : '▼') : '▴▾'}
                  </span>
                </th>
              ))}
            </tr>
            <tr className="qe-rt-filter">
              <th />
              {columns.map((_, i) => (
                <th key={i}>
                  <input
                    placeholder="フィルター…"
                    value={filters[i] ?? ''}
                    onChange={(e) => {
                      setFilters((prev) => ({ ...prev, [i]: e.target.value }));
                      setPage(0);
                    }}
                  />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {pageRows.map((r, ri) => (
              <tr key={start + ri}>
                <td className="qe-rt-idx">{start + ri + 1}</td>
                {columns.map((_, ci) => (
                  <td key={ci} className={numericCols[ci] ? 'num' : ''}>
                    {r[ci] ?? ''}
                  </td>
                ))}
              </tr>
            ))}
            {sorted.length === 0 && (
              <tr>
                <td className="qe-rt-empty" colSpan={columns.length + 1}>
                  結果がありません
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <div className="qe-rt-footer">
        <span>
          {rangeLabel}
          {totalLabel}
        </span>
        {hasMore && (
          <button className="btn sm ghost" onClick={onLoadMore} disabled={isFetchingMore}>
            {isFetchingMore ? '読み込み中…' : 'さらに読み込む'}
          </button>
        )}
        <span className="qe-rt-pager">
          <button
            onClick={() => setPage(Math.max(0, clampedPage - 1))}
            disabled={clampedPage === 0}
          >
            ‹
          </button>
          <span>
            {clampedPage + 1} / {pageCount}
          </span>
          <button
            onClick={() => setPage(Math.min(pageCount - 1, clampedPage + 1))}
            disabled={clampedPage >= pageCount - 1}
          >
            ›
          </button>
        </span>
        {footerRight}
      </div>
    </div>
  );
}
