// Cost Explorer 専用のクロス集計テーブル。
// 縦軸に GroupBy の集計結果 (サービス名等)、横軸に日付ごとの費用を並べる。
// 列数が日付範囲に応じて可変なため、ColumnDef<T>[] 前提の DataTable では表現できず専用実装とする。
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

export function CostCrossTable({ categories, rows }: CostCrossTableProps) {
  return (
    <div className="table-wrap">
      <table className="dt cost-cross-table">
        <thead>
          <tr>
            <th style={{ textAlign: 'left' }}>Group</th>
            <th style={{ textAlign: 'right' }}>Total</th>
            {categories.map((c) => (
              <th key={c} style={{ textAlign: 'right' }}>
                {c}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.group}>
              <td className="primary truncate">{r.group}</td>
              <td
                style={{
                  textAlign: 'right',
                  fontFamily: 'var(--font-mono)',
                  fontWeight: 600,
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
