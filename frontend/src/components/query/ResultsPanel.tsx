// 下ペインのタブシェル (結果 / 履歴 / 保存クエリ / スニペット) とステータス表示スロット
import type { ReactNode } from 'react';
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';

export type ResultsTabKey = 'results' | 'history' | 'saved' | 'snippets';

export interface ResultsPanelProps {
  active: ResultsTabKey;
  onChange: (key: ResultsTabKey) => void;
  // 履歴タブの表示名 (BigQuery: 履歴 / Athena: 実行履歴)
  historyLabel: string;
  status?: ReactNode;
  children: ReactNode;
}

const TAB_DEFS: Array<{
  key: ResultsTabKey;
  label: (t: TFunction, historyLabel: string) => string;
}> = [
  { key: 'results', label: (t) => t('resultsPanel.tabs.results') },
  { key: 'history', label: (_t, h) => h },
  { key: 'saved', label: (t) => t('resultsPanel.tabs.saved') },
  { key: 'snippets', label: (t) => t('resultsPanel.tabs.snippets') },
];

export function ResultsPanel({
  active,
  onChange,
  historyLabel,
  status,
  children,
}: ResultsPanelProps) {
  const { t } = useTranslation('query');
  return (
    <div className="qe-panel qe-results">
      <div className="qe-results-tabs">
        {TAB_DEFS.map((def) => (
          <button
            key={def.key}
            className={active === def.key ? 'active' : ''}
            onClick={() => onChange(def.key)}
          >
            {def.label(t, historyLabel)}
          </button>
        ))}
        <span className="qe-results-status">{status}</span>
      </div>
      <div className="qe-results-body">{children}</div>
    </div>
  );
}
