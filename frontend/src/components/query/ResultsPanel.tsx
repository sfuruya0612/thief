// 下ペインのタブシェル (結果 / 履歴 / 保存クエリ / スニペット) とステータス表示スロット
import type { ReactNode } from 'react';

export type ResultsTabKey = 'results' | 'history' | 'saved' | 'snippets';

export interface ResultsPanelProps {
  active: ResultsTabKey;
  onChange: (key: ResultsTabKey) => void;
  // 履歴タブの表示名 (BigQuery: 履歴 / Athena: 実行履歴)
  historyLabel: string;
  status?: ReactNode;
  children: ReactNode;
}

const TAB_DEFS: Array<{ key: ResultsTabKey; label: (historyLabel: string) => string }> = [
  { key: 'results', label: () => '結果' },
  { key: 'history', label: (h) => h },
  { key: 'saved', label: () => '保存クエリ' },
  { key: 'snippets', label: () => 'スニペット' },
];

export function ResultsPanel({
  active,
  onChange,
  historyLabel,
  status,
  children,
}: ResultsPanelProps) {
  return (
    <div className="qe-panel qe-results">
      <div className="qe-results-tabs">
        {TAB_DEFS.map((t) => (
          <button
            key={t.key}
            className={active === t.key ? 'active' : ''}
            onClick={() => onChange(t.key)}
          >
            {t.label(historyLabel)}
          </button>
        ))}
        <span className="qe-results-status">{status}</span>
      </div>
      <div className="qe-results-body">{children}</div>
    </div>
  );
}
