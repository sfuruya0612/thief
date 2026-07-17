// スキーマツリーの表示部品 (検索ボックス + ツリー行 + フッターヒント)。
// データの取得と組み立ては各ビュー (BigQueryView / AthenaView) の責務。
import type { ReactNode } from 'react';

export interface SchemaTreePanelProps {
  search: string;
  onSearch: (value: string) => void;
  footer: ReactNode;
  children: ReactNode;
}

export function SchemaTreePanel({ search, onSearch, footer, children }: SchemaTreePanelProps) {
  return (
    <div className="qe-panel qe-schema">
      <div className="qe-schema-search">
        <input
          placeholder="テーブルを検索…"
          value={search}
          onChange={(e) => onSearch(e.target.value)}
        />
      </div>
      <div className="qe-schema-tree">{children}</div>
      <div className="qe-schema-footer">{footer}</div>
    </div>
  );
}

export interface SchemaTreeRowProps {
  level: 0 | 1 | 2;
  label: string;
  badge?: string;
  // expandable を undefined にすると矢印スペース自体を描画しない (カラム行)
  expandable?: boolean;
  expanded?: boolean;
  selected?: boolean;
  partition?: boolean;
  title?: string;
  onClick?: (altKey: boolean) => void;
}

export function SchemaTreeRow({
  level,
  label,
  badge,
  expandable,
  expanded,
  selected,
  partition,
  title,
  onClick,
}: SchemaTreeRowProps) {
  const classes = [
    'qe-tree-row',
    `lv${level}`,
    selected ? 'selected' : '',
    partition ? 'partition' : '',
  ]
    .filter(Boolean)
    .join(' ');
  return (
    <div className={classes} title={title ?? label} onClick={(e) => onClick?.(e.altKey)}>
      {expandable !== undefined && (
        <span className="qe-tree-arrow">{expandable ? (expanded ? '▾' : '▸') : ''}</span>
      )}
      <span className="qe-tree-label">{label}</span>
      {badge && <span className="qe-tree-badge">{badge}</span>}
    </div>
  );
}
