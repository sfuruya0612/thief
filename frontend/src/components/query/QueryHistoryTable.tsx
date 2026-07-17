// 実行履歴タブのテーブル。状態ピル / クエリ / 実行時間 / スキャン量 / 実行日時 / アクション。
import type { QueryHistoryRow } from '../../types/query';
import { formatBytes } from '../tables/columns';
import { StatePill } from './StatePill';

export interface QueryHistoryTableProps {
  items: QueryHistoryRow[];
  // バイト数列の見出し (BigQuery: 処理量 / Athena: スキャン量)
  bytesLabel: string;
  formatDuration: (ms: number) => string;
  onOpen: (item: QueryHistoryRow) => void;
  onRerun: (item: QueryHistoryRow) => void;
  isLoading?: boolean;
}

export function QueryHistoryTable({
  items,
  bytesLabel,
  formatDuration,
  onOpen,
  onRerun,
  isLoading,
}: QueryHistoryTableProps) {
  if (isLoading) {
    return <div className="qe-tab-empty">読み込み中…</div>;
  }
  if (items.length === 0) {
    return <div className="qe-tab-empty">実行履歴がありません</div>;
  }
  return (
    <div className="qe-rt-scroll">
      <table className="qe-rt-table qe-history">
        <thead>
          <tr>
            <th style={{ width: 110 }}>State</th>
            <th>Query</th>
            <th className="num" style={{ width: 100 }}>
              実行時間
            </th>
            <th className="num" style={{ width: 110 }}>
              {bytesLabel}
            </th>
            <th style={{ width: 110 }}>実行日時</th>
            <th style={{ width: 120 }} />
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.id}>
              <td>
                <StatePill state={item.state} label={item.stateLabel} />
              </td>
              <td className="qe-history-sql" title={item.sql}>
                {item.sql}
              </td>
              <td className="num">{item.elapsedMs > 0 ? formatDuration(item.elapsedMs) : '–'}</td>
              <td className="num">{item.bytes > 0 ? formatBytes(item.bytes) : '–'}</td>
              <td>{item.startedAt || '–'}</td>
              <td className="qe-history-actions">
                <button onClick={() => onOpen(item)}>開く</button>
                <span> · </span>
                <button onClick={() => onRerun(item)}>再実行</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
