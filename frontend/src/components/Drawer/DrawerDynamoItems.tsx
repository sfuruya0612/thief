// DynamoDB テーブルの Item を Key-Value 指定で検索する Drawer タブ
// 初期表示はプレビュー (Scan Limit:10)、PK (+SK) を指定すると Query (Limit:10) に切り替わる。
// 検索時の負荷とコストを最小化するため、明示検索は必ず Query を使う (バックエンド側で保証)。
// PK/SK に加えて、任意の属性名 + 値による絞り込み (FilterExpression) も PK/SK と併用できる。
import { useMemo, useState } from 'react';
import { useDynamoItems, useDynamoSchema } from '../../api/queries';

const inputStyle = {
  height: 26,
  padding: '0 10px',
  background: 'var(--bg-1)',
  border: '1px solid var(--line-2)',
  borderRadius: 'var(--radius)',
  fontSize: 12,
  color: 'var(--text-1)',
} as const;

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
        <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>
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
            <div style={{ padding: 20, color: 'var(--text-3)' }}>Loading…</div>
          ) : (
            <div className="table-wrap">
              <table className="dt">
                <thead>
                  <tr>
                    {columns.map((c) => (
                      <th key={c}>{c}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row, i) => (
                    <tr key={i}>
                      {columns.map((c) => (
                        <td key={c} style={{ fontFamily: 'var(--font-mono)' }}>
                          {formatItemValue(row[c])}
                        </td>
                      ))}
                    </tr>
                  ))}
                  {rows.length === 0 && (
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
