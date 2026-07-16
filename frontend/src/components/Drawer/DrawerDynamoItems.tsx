// DynamoDB テーブルの Item を Key-Value 指定で検索する Drawer タブ
// 初期表示はプレビュー (Scan)、PK (+SK) を指定すると Query に切り替わる。
// 検索時の負荷とコストを最小化するため、明示検索は必ず Query を使う (バックエンド側で保証)。
// PK/SK に加えて、任意の属性名 + 値による絞り込み (FilterExpression) も PK/SK と併用できる。
// 取得件数は LIMIT_OPTIONS から選択でき、未選択時は DEFAULT_LIMIT (10件)。
import { useMemo, useState } from 'react';
import { useDynamoItems, useDynamoSchema } from '../../api/queries';
import { useColumnResize } from '../../hooks/useColumnResize';
import { DrawerLoading } from './DrawerLoading';

const inputStyle = {
  height: 26,
  padding: '0 10px',
  background: 'var(--bg-1)',
  border: '1px solid var(--line-2)',
  borderRadius: 'var(--radius)',
  fontSize: 12,
  color: 'var(--text-1)',
} as const;

const LIMIT_OPTIONS = [10, 50, 100] as const;
const DEFAULT_LIMIT: (typeof LIMIT_OPTIONS)[number] = 10;

export interface DrawerDynamoItemsProps {
  profile: string;
  region: string;
  table: string;
}

export function DrawerDynamoItems({ profile, region, table }: DrawerDynamoItemsProps) {
  const { data: schema, isLoading: schemaLoading } = useDynamoSchema(profile, region, table);
  const [pkInput, setPkInput] = useState('');
  const [skInput, setSkInput] = useState('');
  const [attrNameInput, setAttrNameInput] = useState('');
  const [attrValueInput, setAttrValueInput] = useState('');
  const [limit, setLimit] = useState<(typeof LIMIT_OPTIONS)[number]>(DEFAULT_LIMIT);
  // 検索実行済みの値のみクエリに渡す (入力中は検索しない)
  const [submittedPk, setSubmittedPk] = useState('');
  const [submittedSk, setSubmittedSk] = useState('');
  const [submittedAttrName, setSubmittedAttrName] = useState('');
  const [submittedAttrValue, setSubmittedAttrValue] = useState('');

  const { data: items, isLoading: itemsLoading } = useDynamoItems(profile, region, table, {
    pkValue: submittedPk || undefined,
    skValue: submittedSk || undefined,
    attrName: submittedAttrName || undefined,
    attrValue: submittedAttrValue || undefined,
    limit,
  });

  const isPreview = !submittedPk && !submittedAttrName;
  const rows = useMemo(() => items ?? [], [items]);
  const columns = useMemo(() => {
    const keys = new Set<string>();
    for (const row of rows) {
      Object.keys(row).forEach((k) => keys.add(k));
    }
    return Array.from(keys);
  }, [rows]);

  // 列ごとのフィルター入力値 (取得済み結果に対するクライアント側の絞り込み)
  const [colFilters, setColFilters] = useState<Record<string, string>>({});
  const filteredRows = useMemo(() => {
    const activeCols = columns.filter((c) => colFilters[c]?.trim());
    if (activeCols.length === 0) return rows;
    return rows.filter((row) =>
      activeCols.every((c) => {
        const v = row[c];
        const text = v == null ? '' : typeof v === 'object' ? JSON.stringify(v) : String(v);
        return text.toLowerCase().includes(colFilters[c].trim().toLowerCase());
      }),
    );
  }, [rows, columns, colFilters]);

  const { colWidths, theadRowRef, startColResize } = useColumnResize();

  const handleSearch = () => {
    setSubmittedPk(pkInput.trim());
    setSubmittedSk(skInput.trim());
    setSubmittedAttrName(attrNameInput.trim());
    setSubmittedAttrValue(attrValueInput.trim());
  };

  const handleClear = () => {
    setPkInput('');
    setSkInput('');
    setAttrNameInput('');
    setAttrValueInput('');
    setSubmittedPk('');
    setSubmittedSk('');
    setSubmittedAttrName('');
    setSubmittedAttrValue('');
  };

  const pkName = schema?.table.partitionKey.name ?? 'PK';
  const skName = schema?.table.sortKey?.name;

  return (
    <div className="section">
      <h3>Items</h3>
      {schemaLoading ? (
        <DrawerLoading />
      ) : (
        <>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
            <input
              style={inputStyle}
              placeholder={pkName}
              value={pkInput}
              onChange={(e) => setPkInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            />
            {skName && (
              <input
                style={inputStyle}
                placeholder={skName}
                value={skInput}
                onChange={(e) => setSkInput(e.target.value)}
                onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
              />
            )}
            <input
              style={inputStyle}
              placeholder="attribute name"
              value={attrNameInput}
              onChange={(e) => setAttrNameInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            />
            <input
              style={inputStyle}
              placeholder="attribute value"
              value={attrValueInput}
              onChange={(e) => setAttrValueInput(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
            />
            <select
              style={inputStyle}
              value={limit}
              onChange={(e) => setLimit(Number(e.target.value) as (typeof LIMIT_OPTIONS)[number])}
            >
              {LIMIT_OPTIONS.map((n) => (
                <option key={n} value={n}>
                  {n} 件
                </option>
              ))}
            </select>
            <button
              className="btn sm"
              onClick={handleSearch}
              disabled={!pkInput.trim() && !attrNameInput.trim()}
            >
              Search
            </button>
            {!isPreview && (
              <button className="btn sm ghost" onClick={handleClear}>
                Clear
              </button>
            )}
          </div>
          <div style={{ color: 'var(--text-3)', fontSize: 12, marginBottom: 8 }}>
            {isPreview
              ? `Preview (最大 ${rows.length} 件、Scan)`
              : `${submittedPk ? 'Query' : 'Scan'} 結果 (最大 ${rows.length} 件)`}
          </div>
          {itemsLoading ? (
            <DrawerLoading />
          ) : (
            <div className="table-wrap">
              <table className="dt">
                <colgroup>
                  {columns.map((c) => (
                    <col key={c} style={{ width: colWidths[c] }} />
                  ))}
                </colgroup>
                <thead>
                  <tr ref={theadRowRef}>
                    {columns.map((c) => (
                      <th key={c} data-col-key={c} style={{ position: 'relative' }}>
                        {c}
                        <span
                          className="col-resize-handle"
                          onPointerDown={startColResize(c)}
                          title="Drag to resize column"
                        />
                      </th>
                    ))}
                  </tr>
                  <tr className="dt-filter-row">
                    {columns.map((c) => (
                      <th key={c}>
                        <input
                          className="dt-col-filter"
                          value={colFilters[c] ?? ''}
                          placeholder="フィルター…"
                          onClick={(e) => e.stopPropagation()}
                          onChange={(e) =>
                            setColFilters((prev) => ({ ...prev, [c]: e.target.value }))
                          }
                        />
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {filteredRows.map((row, i) => (
                    <tr key={i}>
                      {columns.map((c) => (
                        <td key={c} style={{ fontFamily: 'var(--font-mono)' }}>
                          {formatItemValue(row[c])}
                        </td>
                      ))}
                    </tr>
                  ))}
                  {filteredRows.length === 0 && (
                    <tr>
                      <td
                        colSpan={Math.max(columns.length, 1)}
                        style={{ textAlign: 'center', padding: 40, color: 'var(--text-3)' }}
                      >
                        No items found
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  );
}

function formatItemValue(v: unknown): string {
  if (v === undefined) return '';
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}
