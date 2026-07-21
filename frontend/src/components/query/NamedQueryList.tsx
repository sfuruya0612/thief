// 保存クエリ / スニペットタブの一覧表示。開く (新規タブ) と削除、任意で挿入を提供する。
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { NamedQuery } from '../../types/query';
import { formatTimestampShort } from '../../lib/queryFormat';

export interface NamedQueryListProps {
  items: NamedQuery[];
  emptyText: string;
  onOpen: (query: NamedQuery) => void;
  onInsert?: (query: NamedQuery) => void;
  onDelete: (id: string) => void;
  header?: ReactNode;
}

export function NamedQueryList({
  items,
  emptyText,
  onOpen,
  onInsert,
  onDelete,
  header,
}: NamedQueryListProps) {
  const { t } = useTranslation('query');
  return (
    <div className="qe-named-list">
      {header && <div className="qe-named-header">{header}</div>}
      {items.map((q) => (
        <div key={q.id} className="qe-named-item">
          <div className="qe-named-body" onClick={() => onOpen(q)} title={q.sql}>
            <b>{q.name}</b>
            <span className="qe-named-sql">{q.sql}</span>
          </div>
          <div className="qe-named-actions">
            <span className="qe-named-date">{formatTimestampShort(q.updatedAt)}</span>
            <button onClick={() => onOpen(q)}>{t('namedQueryList.open')}</button>
            {onInsert && <button onClick={() => onInsert(q)}>{t('namedQueryList.insert')}</button>}
            <button onClick={() => onDelete(q.id)}>{t('namedQueryList.delete')}</button>
          </div>
        </div>
      ))}
      {items.length === 0 && <div className="qe-tab-empty">{emptyText}</div>}
    </div>
  );
}
